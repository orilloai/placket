# `make ci` is placket's one entry point for checks; GitHub runs it on every push and pull request.
#
# The guardrail checks run first, each as its own step named for the rule it enforces (AGENTS.md).
# Every line a check prints starts with the rule's name: "<rule>: ok" when it passes, and
# "<rule>: <file>:<line>: <code>" for each offending line when it refuses. Then every Go module's
# format, vet and tests, and every tests/test_*.sh.
.PHONY: ci core-no-deps core-imports core-no-world core-no-panic modules scripts
ci: core-no-deps core-imports core-no-world core-no-panic modules scripts

# GUARD_SCAN is an awk program. For each line of its input files whose code matches the ERE `re`, it
# prints "<rule>: <file>:<line>: <the line>". The code is the line with its comments removed: a //
# comment to the end of the line, and a /* */ comment, which may span lines and becomes one space.
# Comment markers inside a "...", '...' or `...` literal are text, and literals stay in the code. The
# state of a /* */ comment or a `...` raw string carries from one line to the next within a file.
define GUARD_SCAN
function code(s,    i, c, out) {
	out = ""
	if (q != "`") q = ""
	for (i = 1; i <= length(s); i++) {
		c = substr(s, i, 1)
		if (bc) {
			if (c == "*" && substr(s, i + 1, 1) == "/") { bc = 0; i++; out = out " " }
			continue
		}
		if (q == "") {
			if (c == "/" && substr(s, i + 1, 1) == "/") break
			if (c == "/" && substr(s, i + 1, 1) == "*") { bc = 1; i++; continue }
			if (c == "\"" || c == "'" || c == "`") q = c
		} else if (c == "\\" && q != "`") {
			out = out c; i++; c = substr(s, i, 1)
		} else if (c == q) {
			q = ""
		}
		out = out c
	}
	return out
}
FNR == 1 { bc = 0; q = "" }
code($$0) ~ re {
	t = $$0; sub(/^[ \t]+/, "", t)
	print rule ": " FILENAME ":" FNR ": " t
}
endef
export GUARD_SCAN

# guard,<files>,<ERE>,<why>: the body of a check named for its target. <files> are find arguments under
# core/. Prints "<rule>: ok", or every offending line in file and line order and then the rule's reason.
# A literal comma in an argument is written $(comma); the ERE holds no single quote and no backslash.
define guard
	@set -e; \
	if [ ! -d core ]; then echo "$@: core/: missing; the core is its own module there"; exit 1; fi; \
	hits=$$(find core $(1) -type f -exec awk -v rule='$@' -v re='$(2)' "$$GUARD_SCAN" {} +); \
	if [ -z "$$hits" ]; then echo "$@: ok"; exit 0; fi; \
	printf '%s\n' "$$hits" | sort -t: -k2,2 -k3,3n; echo "$@: refused: $(3) (AGENTS.md, $@)"; exit 1
endef

comma := ,

# Every Go file under core/, test files and testdata included.
CORE_ALL = -name '*.go'
# The Go files under core/ that are built into the core: no _test.go file, nothing under testdata/.
CORE_SRC = -name '*.go' ! -name '*_test.go' ! -path '*/testdata/*'

# core-imports: an import path of adapters/, cmd/ or recipes/, in either quote form Go allows.
IMPORTS_RE = ["`]github[.]com/orilloai/placket/(adapters|cmd|recipes)(/[^"`]*)?["`]
# core-no-deps: a require directive, single or block.
REQUIRE_RE = ^[ \t]*require([ \t(]|$$)
# core-no-world: by import path, under any name, the packages for randomness, the network, the process
# and its files and environment, logs, and memory (crypto/rand, math/rand, net, os, syscall, io/ioutil,
# log, runtime, unsafe, and those below them); by name, the clock (time.Now and the calls that wait on
# or measure against it), os.Getenv and os.Open, and printing (fmt.Print*, print, println).
WORLD_PKGS = crypto/rand|math/rand(/v2)?|net(/[^"`]*)?|os(/[^"`]*)?|syscall|io/ioutil|log(/[^"`]*)?|runtime(/[^"`]*)?|unsafe
WORLD_CALLS = time[.](Now|Since|Until|After|AfterFunc|Tick|NewTimer|NewTicker|Sleep)|os[.]Getenv|os[.]Open|fmt[.]Print
WORLD_RE = (^|[^A-Za-z0-9_])($(WORLD_CALLS))|(^|[^A-Za-z0-9_.])print(ln)?[ \t]*[(]|["`]($(WORLD_PKGS))["`]
# core-no-panic: panic( as a word.
PANIC_RE = (^|[^A-Za-z0-9_])panic[ \t]*[(]

core-imports:
	$(call guard,$(CORE_ALL),$(IMPORTS_RE),nothing under core/ imports adapters/$(comma) cmd/ or recipes/)

core-no-deps:
	@if [ ! -f core/go.mod ]; then echo "$@: core/go.mod: missing; the core is its own module"; exit 1; fi
	$(call guard,-path core/go.mod,$(REQUIRE_RE),core/go.mod requires no module)

core-no-world:
	$(call guard,$(CORE_SRC),$(WORLD_RE),no non-test code in core/ reads the clock$(comma) randomness$(comma) the environment or files$(comma) or touches the network$(comma) the process or memory$(comma) or prints)

core-no-panic:
	$(call guard,$(CORE_SRC),$(PANIC_RE),no non-test code in core/ calls panic)

# Every Go module's format, vet and tests. A module under a testdata/ directory is a fixture and is
# skipped, as is a module with no Go packages yet (go vet and go test refuse an empty pattern). A module
# whose packages cannot be listed fails here; it is never taken for an empty one.
modules:
	@set -e; for m in $$(find . -name go.mod -not -path './.git/*' -not -path '*/testdata/*' -exec dirname {} \; | sort); do \
		pkgs=$$(cd "$$m" && go list ./...) || { echo "== $$m: go list failed"; exit 1; }; \
		if [ -z "$$pkgs" ]; then echo "== $$m: no Go packages; skipped"; continue; fi; \
		echo "== $$m"; \
		unformatted=$$(cd "$$m" && find . -name '*.go' -not -path '*/testdata/*' -exec gofmt -l {} +); \
		if [ -n "$$unformatted" ]; then echo "gofmt: $$m: not formatted:" $$unformatted; exit 1; fi; \
		(cd "$$m" && go vet ./... && go test -count=1 ./...); done

# The repository's own shell tests.
scripts:
	@set -e; for t in tests/test_*.sh; do [ -e "$$t" ] || continue; echo "== $$t"; sh "$$t"; done
