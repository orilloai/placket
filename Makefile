# `make ci` is placket's one entry point for checks; GitHub runs it on every pull request and every push to main.
#
# Every step is a target of its own, named for the rule it enforces (docs/rules/) or for what it does.
# The output contract:
# - A check of the tree prints "<rule>: ok" when it passes. When it refuses, every line it prints
#   starts "<rule>: <file>", with ":<line>" where the fault has a line, or is an indented detail line
#   under one, and its last line is "<rule>: refused: <why> (docs/rules/<file>.md#<rule>)", which links
#   the rule's heading. schemas-hold first
#   prints one count line per schema that holds, "schemas-hold: <schema>: ok, <n> accepted, <n> refused".
# - A toolchain step prints "<step>: ok", or "<step>: <module>: refused" after the tool's own output.
# - Every step then prints its time on a line of its own, "time: <step>: <n>s", whether it passed or
#   refused. No time fails a step: the workflow's timeout is the limit.
# - A make ci started inside another prints "ci-stays-fast: refused: make ci inside make ci" and stops
#   before any step.
#
# First the standard library's packages the modules import, compiled once for every step that compiles
# (build). The checks of the tree are the rules of one Go program, tools/guard/
# (docs/adr/0007-one-checker.md), built once (guard) and run once per rule. Then every Go module's
# format (modules), the toolchain steps, the shared test run they read (tests), the conformance runner over vectors/
# (runner-no-silent-pass) and every tests/test_*.sh (scripts). ci's prerequisites are written out as
# literal names: machine-rules-in-ci refuses a ci that leaves out a rule of the checker or a step.
.DEFAULT_GOAL := ci
CHECKS = core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold \
	vectors-independent constants-once product-words-once claims-stop-at-crossing main-is-thin recipes-core-only root-module \
	machine-rules-in-ci ci-stays-fast no-hidden-text agents-md-short docs-tree-holds
TOOLCHAIN = vet lint vulnerabilities secrets race fuzz-seeds coverage-reported
.PHONY: ci build guard $(CHECKS) modules $(TOOLCHAIN) tests runner-no-silent-pass scripts
ci: build guard core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold \
	vectors-independent constants-once product-words-once claims-stop-at-crossing main-is-thin recipes-core-only root-module \
	machine-rules-in-ci ci-stays-fast no-hidden-text agents-md-short docs-tree-holds modules vet lint vulnerabilities secrets tests race fuzz-seeds \
	coverage-reported runner-no-silent-pass scripts

# ci-stays-fast, the marker: make ci exports PLACKET_IN_CI=1 to every step. A make whose goals include
# ci, or that has no goal and so runs ci, stops here when the marker is already set, before any step and
# whatever -k says: a test that runs make ci inside make ci would run every step again. The checker
# refuses a test or script that clears the marker.
ci: export PLACKET_IN_CI = 1
# No step of make ci reaches a judge's host: every step sees the judges' key empty, which every reader
# of it takes for no key (docs/adr/0023-judges.md).
ci: export PLACKET_JEV_KEY =
# CI_JOBS is how much runs side by side: by default the host's processor count, and at least 2.
CI_JOBS ?= $(shell n=$$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2); [ "$$n" -ge 2 ] 2>/dev/null || n=2; echo $$n)
ifneq ($(filter ci,$(or $(MAKECMDGOALS),ci)),)
ifneq ($(PLACKET_IN_CI),)
$(info ci-stays-fast: refused: make ci inside make ci)
$(error PLACKET_IN_CI is set, so a make ci is already running above this one (docs/rules/changes.md#ci-stays-fast))
endif
# ci-stays-fast, the jobs: make ci runs its steps side by side, CI_JOBS at a time, each step's
# output held back and printed whole when the step ends, so that its lines and its time line stay
# together. Every step still runs once, and a step that reads another's output names it as a
# prerequisite. `make ci CI_JOBS=1` runs them one by one.
MAKEFLAGS += -j$(CI_JOBS) --output-sync=target
endif

# TURNS opens, on descriptor 9, a queue of $$turns turns in the directory given: a job of a step
# takes one before it starts (TAKE) and gives it back when it ends (GIVE), so that a step runs no
# more of its jobs at a time than it has turns, on a host of any size. What a job runs does not
# inherit the descriptor.
TURNS = mkfifo "$(1)/turns"; exec 9<>"$(1)/turns"; n=0; while [ "$$n" -lt "$$turns" ]; do echo >&9; n=$$((n + 1)); done
TAKE = read -r _ <&9
GIVE = echo >&9

# Every step's recipe is one shell line that starts with $(START) and ends with $(END), its body in
# between, in a subshell, so that its time is printed after its output whatever its status.
START = t0=$$(date +%s); (
END = ); rc=$$?; echo "time: $@: $$(($$(date +%s) - t0))s"; exit $$rc

# WORK is this checkout's scratch directory, outside the tree: the checker's binary, the shared test
# run's output and the shell tests' output. Each run of guard, tests and scripts writes its part afresh
# before anything reads it. Its path is fixed, so each of them first makes it with mode 700
# (OWN_WORK), and uses it only when it is a directory, not a symbolic link, owned by the user running
# make and writable by no one else; otherwise it stops.
WORK := $(or $(TMPDIR),/tmp)/placket-make-$(shell printf '%s' '$(CURDIR)' | cksum | cut -d' ' -f1)
GUARD = $(WORK)/guard
OWN_WORK = mkdir -m 700 "$(WORK)" 2>/dev/null || :; \
	if [ -L "$(WORK)" ] || [ ! -d "$(WORK)" ] || [ ! -O "$(WORK)" ]; then \
		echo "$@: $(WORK): not a directory of this user's own; remove it"; exit 1; fi; \
	case $$(ls -ld "$(WORK)") in d????-??-*) ;; *) \
		echo "$@: $(WORK): writable by others; remove it"; exit 1;; esac

# build: the standard library's packages that some module's packages or tests import, compiled once,
# as they are and with the race detector, into the build cache, before any step that compiles. The
# steps run side by side, and two go commands that need one package at once would each compile it.
# It reads the modules with go list -e, so a module that cannot be listed is left to modules and the
# toolchain steps to name; it fails only when the standard library does not build.
build:
	@$(START) set -e; \
		std=$$(for m in $$($(MODULES)); do (cd "$$m" && go list -e -deps -test -f '{{if .Standard}}{{.ImportPath}}{{end}}' ./... 2>/dev/null) || :; done | sort -u); \
		if [ -n "$$std" ]; then go build $$std && go build -race $$std || { echo "$@: std: does not build"; exit 1; }; fi; \
		echo "$@: ok" $(END)

# guard: the checker, built once from tools/guard/, its own module.
guard: build
	@$(START) set -e; $(OWN_WORK); \
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

vet: build
	@$(START) $(call each_module,go vet ./...) $(END)

# lint keeps the linter's own cache of results inside Go's build cache, unless STATICCHECK_CACHE is
# already set, so that whatever keeps the build cache from one run to the next (the workflow's
# setup-go cache) keeps the linter's too. Its entries are keyed on their inputs, as Go's are.
lint: build
	@$(START) c=$$(go env GOCACHE); case $$c in /*) export STATICCHECK_CACHE="$${STATICCHECK_CACHE:-$$c/staticcheck}";; esac; \
		$(call each_module,go run $(STATICCHECK) ./...) $(END)

vulnerabilities: build
	@$(START) $(call each_module,go run $(GOVULNCHECK) ./...) $(END)

# secrets: the secret scan reads every file of the tree but .git/, and names each finding by file, line
# and the kind of secret, never the secret itself.
secrets: build
	@$(START) set -e; r=$$(mktemp); trap 'rm -f "$$r"' EXIT; \
		if go run $(GITLEAKS) dir --no-banner --redact --log-level error --report-format json --report-path "$$r" . ; then echo "$@: ok"; exit 0; fi; \
		if [ -s "$$r" ]; then jq -r '.[] | "$@: \(.File | sub("^[.]/"; "")):\(.StartLine): \(.RuleID)"' "$$r"; fi; \
		echo "$@: refused: no secret is committed (docs/rules/checks.md#secrets)"; exit 1 $(END)

# tests: one test run per module with packages, with the race detector and coverage, which also runs
# every fuzz test on its seed corpus (the f.Add seeds and testdata/fuzz/), as go test does without
# -fuzz. The modules' runs go side by side, half of CI_JOBS at a time and at least two, since each run
# uses every processor itself, and their lines are printed in the modules' order once
# every run has ended. -count=1 keeps a cached result from standing in for a run. Its output, coverage
# profile and status go to $(WORK)/tests/, for race, fuzz-seeds and coverage-reported to read; each of them runs
# it first, so run alone, each makes the run itself, and in ci it runs once. It prints one line per
# module and fails only when a module's packages cannot be listed: the steps that read it judge it.
TESTS = $(WORK)/tests
tests: build
	@$(START) set -e; $(OWN_WORK); rm -rf "$(TESTS)"; mkdir "$(TESTS)"; : >"$(TESTS)/modules"; : >"$(TESTS)/run"; \
		for m in $$($(MODULES)); do \
			pkgs=$$(cd "$$m" && go list ./...) || { echo "$@: $$m: go list failed"; exit 1; }; \
			[ -n "$$pkgs" ] || continue; \
			echo "$$m $$(printf '%s' "$$m" | tr -c 'A-Za-z0-9\n' _)" >>"$(TESTS)/run"; \
		done; \
		turns=$$((($(CI_JOBS) + 1) / 2)); [ "$(CI_JOBS)" -lt 2 ] || [ "$$turns" -ge 2 ] || turns=2; $(call TURNS,$(TESTS)); \
		while read -r m k; do $(TAKE); \
			{ rc=0; (cd "$$m" && go test -race -count=1 -v -coverprofile="$(TESTS)/$$k.cover" ./...) >"$(TESTS)/$$k.out" 2>&1 9<&- || rc=$$?; \
				echo "$$rc" >"$(TESTS)/$$k.rc"; $(GIVE); } & \
		done <"$(TESTS)/run"; wait; exec 9<&-; \
		while read -r m k; do \
			rc=$$(cat "$(TESTS)/$$k.rc"); echo "$$m $$k $$rc" >>"$(TESTS)/modules"; \
			if [ "$$rc" -eq 0 ]; then echo "$@: $$m: passed"; else echo "$@: $$m: failed"; fi; \
		done <"$(TESTS)/run"; echo "$@: ok" $(END)

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
runner-no-silent-pass: build
	@$(START) set -e; if [ ! -d core/conformance ]; then echo "$@: core/conformance/: missing; the runner lives there"; exit 1; fi; \
		out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
		if (cd core/conformance && go test -count=1 -run '^TestVectors$$') >"$$out" 2>&1; then \
			grep '^$@: ' "$$out"; echo "$@: ok"; exit 0; fi; \
		cat "$$out"; echo "$@: refused: a green run means every vector ran and passed (docs/rules/tests.md#$@)"; exit 1 $(END)

# The repository's own shell tests, side by side, CI_JOBS at a time. Each one's output is printed
# whole, in the tests' order, under "== <test>", once every test has ended; the step fails when any
# test failed, naming each one that did.
SCRIPTS = $(WORK)/scripts
scripts: build
	@$(START) set -e; $(OWN_WORK); rm -rf "$(SCRIPTS)"; mkdir "$(SCRIPTS)"; i=0; \
		turns=$(CI_JOBS); $(call TURNS,$(SCRIPTS)); \
		for t in tests/test_*.sh; do [ -e "$$t" ] || continue; i=$$((i + 1)); $(TAKE); \
			{ rc=0; sh "$$t" >"$(SCRIPTS)/$$i.out" 2>&1 9<&- || rc=$$?; echo "$$rc" >"$(SCRIPTS)/$$i.rc"; $(GIVE); } & \
		done; wait; exec 9<&-; i=0; failed=; \
		for t in tests/test_*.sh; do [ -e "$$t" ] || continue; i=$$((i + 1)); \
			echo "== $$t"; cat "$(SCRIPTS)/$$i.out"; [ "$$(cat "$(SCRIPTS)/$$i.rc")" -eq 0 ] || failed="$$failed $$t"; \
		done; if [ -n "$$failed" ]; then echo "$@: failed:$$failed"; exit 1; fi $(END)

# demo: the worked example, ask and log, run by hand: three parties' commands against one window,
# a restart, and the journal verified and exported. Not a step of ci: tests/test_demo.sh runs the
# same script in scripts.
.PHONY: demo
demo:
	@sh scripts/demo.sh

# judge-eval: the judge's basic eval, run by hand with the user's own key in PLACKET_JEV_KEY: every
# case of CASES against the judge config JUDGE, one line per case and the counts (evals/judge/README.md,
# docs/adr/0023-judges.md). Not a step of ci: each case is a request to the judge's host.
CASES ?= evals/judge/cases.jsonl
.PHONY: judge-eval
judge-eval:
	@[ -n "$(JUDGE)" ] || { echo "judge-eval: give JUDGE=<a judge's config>"; exit 2; }; \
		go run ./cmd/judge-eval "$(JUDGE)" "$(CASES)"

# red-green-tests: every test the change adds or changes fails without its code and passes with it,
# and no test is removed or skipped unexplained (docs/rules/tests.md#red-green-tests). Not a step of ci: it
# needs main and scratch worktrees. It judges the commits from the merge base of BASE and HEAD to
# HEAD, so commit first; uncommitted changes are not read. The workflow runs the same program, built
# from main's copy (docs/adr/0013-red-green-tests.md). It then checks outside-commits-signed-off with
# no opener, so only a bot's sign-off is refused (docs/rules/changes.md#outside-commits-signed-off). It
# exits 0 when ok, 1 when either refuses and 3 when it could not judge.
BASE ?= origin/main
.PHONY: red-green-tests
red-green-tests:
	@set -e; $(OWN_WORK); \
		(cd tools/redgreen && go build -o "$(WORK)/redgreen" .) || { echo "$@: error: tools/redgreen does not build"; exit 3; }; \
		b=$$(git merge-base "$(BASE)" HEAD) || { echo "$@: error: no merge base of $(BASE) and HEAD"; exit 3; }; \
		"$(WORK)/redgreen" "$$b" HEAD
