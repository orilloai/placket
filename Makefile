# `make ci` is placket's one entry point for checks; GitHub runs it on every push and pull request.
#
# Every step is a target of its own, named for the rule it enforces (AGENTS.md) or for what it does.
# The output contract:
# - A check of the tree prints "<rule>: ok" when it passes. When it refuses, every line it prints
#   starts "<rule>: <file>", with ":<line>" where the fault has a line, or is an indented detail line
#   under one, and its last line is "<rule>: refused: <why> (AGENTS.md, <rule>)". schemas-hold first
#   prints one count line per schema that holds, "schemas-hold: <schema>: ok, <n> accepted, <n> refused".
# - A toolchain step prints "<step>: ok", or "<step>: <module>: refused" after the tool's own output.
# - Every step then prints its time on a line of its own, "time: <step>: <n>s", whether it passed or
#   refused. No time fails a step: the workflow's timeout is the limit.
# - A make ci started inside another prints "ci-stays-fast: refused: make ci inside make ci" and stops
#   before any step.
#
# The checks of the tree are the rules of one Go program, tools/guard/ (docs/adr/0007-one-checker.md),
# built once (guard) and run once per rule. Then every Go module's format (modules), the toolchain
# steps, the shared test run they read (tests), the conformance runner over vectors/
# (runner-no-silent-pass) and every tests/test_*.sh (scripts). ci's prerequisites are written out as
# literal names: machine-rules-in-ci refuses a ci that leaves out a rule of the checker or a step.
.DEFAULT_GOAL := ci
CHECKS = core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold \
	vectors-independent constants-once product-words-once claims-stop-at-crossing main-is-thin root-module \
	machine-rules-in-ci ci-stays-fast
TOOLCHAIN = vet lint vulnerabilities secrets race fuzz-seeds coverage-reported
.PHONY: ci guard $(CHECKS) modules $(TOOLCHAIN) tests runner-no-silent-pass scripts
ci: guard core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold \
	vectors-independent constants-once product-words-once claims-stop-at-crossing main-is-thin root-module \
	machine-rules-in-ci ci-stays-fast modules vet lint vulnerabilities secrets tests race fuzz-seeds \
	coverage-reported runner-no-silent-pass scripts

# ci-stays-fast, the marker: make ci exports PLACKET_IN_CI=1 to every step. A make whose goals include
# ci, or that has no goal and so runs ci, stops here when the marker is already set, before any step and
# whatever -k says: a test that runs make ci inside make ci would run every step again. The checker
# refuses a test or script that clears the marker.
ci: export PLACKET_IN_CI = 1
ifneq ($(filter ci,$(or $(MAKECMDGOALS),ci)),)
ifneq ($(PLACKET_IN_CI),)
$(info ci-stays-fast: refused: make ci inside make ci)
$(error PLACKET_IN_CI is set, so a make ci is already running above this one (AGENTS.md, ci-stays-fast))
endif
endif

# Every step's recipe is one shell line that starts with $(START) and ends with $(END), its body in
# between, in a subshell, so that its time is printed after its output whatever its status.
START = t0=$$(date +%s); (
END = ); rc=$$?; echo "time: $@: $$(($$(date +%s) - t0))s"; exit $$rc

# WORK is this checkout's scratch directory, outside the tree: the checker's binary and the shared test
# run's output. Each run of guard and tests writes it afresh before anything reads it.
WORK := $(or $(TMPDIR),/tmp)/placket-make-$(shell printf '%s' '$(CURDIR)' | cksum | cut -d' ' -f1)
GUARD = $(WORK)/guard

# guard: the checker, built once from tools/guard/, its own module.
guard:
	@$(START) set -e; mkdir -p "$(WORK)"; \
		(cd tools/guard && go build -o "$(GUARD)" .) || { echo "$@: tools/guard: does not build"; exit 1; }; \
		echo "$@: ok" $(END)

# Each check of the tree runs the built checker for its one rule.
$(CHECKS): guard
	@$(START) "$(GUARD)" $@ $(END)

# MODULES lists every Go module, outside .git/ and every testdata/ directory, in order.
MODULES = find . -name go.mod -not -path './.git/*' -not -path '*/testdata/*' -exec dirname {} \; | sort

# Every Go module's format. A module under a testdata/ directory is a fixture and is skipped. Every
# other module's Go files are format-checked, whatever their build constraints, since the checker reads
# them all. A module with no Go packages on this host is then named as skipped: the toolchain steps
# pass over it (go vet and go test refuse an empty pattern). A module whose packages cannot be listed
# fails here, and in every toolchain step; it is never taken for an empty one.
modules:
	@$(START) set -e; for m in $$($(MODULES)); do \
		pkgs=$$(cd "$$m" && go list ./...) || { echo "== $$m: go list failed"; exit 1; }; \
		unformatted=$$(cd "$$m" && find . -name '*.go' -not -path '*/testdata/*' -exec gofmt -l {} +); \
		if [ -n "$$unformatted" ]; then echo "gofmt: $$m: not formatted:" $$unformatted; exit 1; fi; \
		if [ -z "$$pkgs" ]; then echo "== $$m: no Go packages; skipped"; continue; fi; \
		echo "== $$m"; done $(END)

# The toolchain, in every Go module that has packages, as `modules` finds them: go vet; the linter, the
# vulnerability scan and the secret scan, each run by `go run` at the exact version named here, so that
# no module requires them; and the shared test run, read by race, fuzz-seeds and coverage-reported.
# Each step prints "<step>: ok", or "<step>: <module>: refused" after the tool's own output.
STATICCHECK = honnef.co/go/tools/cmd/staticcheck@v0.8.2
GOVULNCHECK = golang.org/x/vuln/cmd/govulncheck@v1.9.0
GITLEAKS = github.com/zricethezav/gitleaks/v8@v8.30.1

# each_module,<command>: run <command> in each module with Go packages, failing the step on the first
# module where it fails.
define each_module
	set -e; for m in $$($(MODULES)); do \
		pkgs=$$(cd "$$m" && go list ./...) || { echo "$@: $$m: go list failed"; exit 1; }; \
		[ -n "$$pkgs" ] || continue; \
		(cd "$$m" && $(1)) || { echo "$@: $$m: refused"; exit 1; }; \
	done; echo "$@: ok"
endef

vet:
	@$(START) $(call each_module,go vet ./...) $(END)

lint:
	@$(START) $(call each_module,go run $(STATICCHECK) ./...) $(END)

vulnerabilities:
	@$(START) $(call each_module,go run $(GOVULNCHECK) ./...) $(END)

# secrets: the secret scan reads every file of the tree but .git/, and names each finding by file, line
# and the kind of secret, never the secret itself.
secrets:
	@$(START) set -e; r=$$(mktemp); trap 'rm -f "$$r"' EXIT; \
		if go run $(GITLEAKS) dir --no-banner --redact --log-level error --report-format json --report-path "$$r" . ; then echo "$@: ok"; exit 0; fi; \
		if [ -s "$$r" ]; then jq -r '.[] | "$@: \(.File | sub("^[.]/"; "")):\(.StartLine): \(.RuleID)"' "$$r"; fi; \
		echo "$@: refused: no secret is committed (AGENTS.md, machine-rules-in-ci)"; exit 1 $(END)

# tests: one test run per module with packages, with the race detector and coverage, which also runs
# every fuzz test on its seed corpus (the f.Add seeds and testdata/fuzz/), as go test does without
# -fuzz. -count=1 keeps a cached result from standing in for a run. Its output, coverage profile and
# status go to $(WORK)/tests/, for race, fuzz-seeds and coverage-reported to read; each of them runs
# it first, so run alone, each makes the run itself, and in ci it runs once. It prints one line per
# module and fails only when a module's packages cannot be listed: the steps that read it judge it.
TESTS = $(WORK)/tests
tests:
	@$(START) set -e; rm -rf "$(TESTS)"; mkdir -p "$(TESTS)"; : >"$(TESTS)/modules"; \
		for m in $$($(MODULES)); do \
			pkgs=$$(cd "$$m" && go list ./...) || { echo "$@: $$m: go list failed"; exit 1; }; \
			[ -n "$$pkgs" ] || continue; \
			k=$$(printf '%s' "$$m" | tr -c 'A-Za-z0-9\n' _); \
			rc=0; (cd "$$m" && go test -race -count=1 -v -coverprofile="$(TESTS)/$$k.cover" ./...) >"$(TESTS)/$$k.out" 2>&1 || rc=$$?; \
			echo "$$m $$k $$rc" >>"$(TESTS)/modules"; \
			if [ "$$rc" -eq 0 ]; then echo "$@: $$m: passed"; else echo "$@: $$m: failed"; fi; \
		done; echo "$@: ok" $(END)

# SHOWN is an awk program that prints a test run's output without the lines of tests that passed or
# were skipped, and without the lines that only mark a test's start: what is left is the failures,
# with the race detector's reports, and each package's result.
define SHOWN
/^[ \t]*=== (RUN|PAUSE|CONT|NAME)[ \t]/ { next }
/^[ \t]*--- (PASS|SKIP): / { next }
/^(PASS|ok  [ \t])/ { next }
{ print }
endef
export SHOWN

# race: every test of the shared run passed, with no data race; any failure of the run refuses it.
race: tests
	@$(START) set -e; while read -r m k rc; do \
		if [ "$$rc" -ne 0 ]; then awk "$$SHOWN" "$(TESTS)/$$k.out"; echo "$@: $$m: refused"; exit 1; fi; \
	done <"$(TESTS)/modules"; echo "$@: ok" $(END)

# fuzz-seeds: each fuzz test ran on its seed corpus in the shared run and passed. It counts the fuzz
# tests that ran, and refuses a module where one failed, naming it, or where the run failed with no
# test failing, so that its fuzz tests may not have run at all.
fuzz-seeds: tests
	@$(START) set -e; while read -r m k rc; do \
		o="$(TESTS)/$$k.out"; \
		n=$$(grep -c '^=== RUN   Fuzz[^/]*$$' "$$o" || :); \
		if grep -q '^--- FAIL: Fuzz' "$$o" || { [ "$$rc" -ne 0 ] && ! grep -q '^--- FAIL: ' "$$o"; }; then \
			awk "$$SHOWN" "$$o"; echo "$@: $$m: refused"; exit 1; fi; \
		echo "$@: $$m: $$n fuzz tests run on their seed corpora"; \
	done <"$(TESTS)/modules"; echo "$@: ok" $(END)

# coverage-reported: each module's coverage from the shared run, per function and in total; never a
# gate. It refuses only a module whose run wrote no coverage profile at all.
coverage-reported: tests
	@$(START) set -e; while read -r m k rc; do \
		p="$(TESTS)/$$k.cover"; \
		if [ ! -s "$$p" ]; then awk "$$SHOWN" "$(TESTS)/$$k.out"; echo "$@: $$m: refused"; exit 1; fi; \
		echo "$@: $$m:"; (cd "$$m" && go tool cover -func="$$p"); \
	done <"$(TESTS)/modules"; echo "$@: ok" $(END)

# runner-no-silent-pass: the conformance runner over vectors/, as core/conformance's TestVectors. It
# prints every vector's result, every open entry of the coverage list and a count, each line starting
# with the rule's name, and fails unless every vector found passed and the coverage list holds.
runner-no-silent-pass:
	@$(START) set -e; if [ ! -d core/conformance ]; then echo "$@: core/conformance/: missing; the runner lives there"; exit 1; fi; \
		out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
		if (cd core/conformance && go test -count=1 -run '^TestVectors$$') >"$$out" 2>&1; then \
			grep '^$@: ' "$$out"; echo "$@: ok"; exit 0; fi; \
		cat "$$out"; echo "$@: refused: a green run means every vector ran and passed (AGENTS.md, $@)"; exit 1 $(END)

# The repository's own shell tests.
scripts:
	@$(START) set -e; for t in tests/test_*.sh; do [ -e "$$t" ] || continue; echo "== $$t"; sh "$$t"; done $(END)
