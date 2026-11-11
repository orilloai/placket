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
# Before `re` is matched, the spaces around a "." are dropped, and a line that ends in "<name>." lends
# that to the next line with code, so a selector split after its dot reads as one ("time.Now").
# On an import line (an import declaration's own line, or a line of its parenthesised block), the code
# is also matched against the ERE `ire` when it is set, and with esc=1 a backslash anywhere in it is
# refused: an escape in an import path would hide the path from these greps.
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
FNR == 1 { bc = 0; q = ""; imp = 0; lend = "" }
{
	raw = (q == "`")
	c = code($$0)
	il = 0
	if (imp) {
		il = 1
		if (c ~ /[)]/) imp = 0
	} else if (!raw && c ~ /^[ \t]*import([ \t("`]|$$)/) {
		il = 1
		if (c ~ /^[ \t]*import[ \t]*[(]/ && c !~ /[)]/) imp = 1
	}
	m = lend c
	gsub(/[ \t]*[.][ \t]*/, ".", m)
	if (c ~ /[^ \t]/) lend = match(c, /[A-Za-z_][A-Za-z0-9_]*[ \t]*[.][ \t]*$$/) ? substr(c, RSTART) : ""
	if (m ~ re || (il && ((ire != "" && c ~ ire) || (esc && index(c, "\\"))))) {
		t = $$0; sub(/^[ \t]+/, "", t)
		print rule ": " FILENAME ":" FNR ": " t
	}
}
endef
export GUARD_SCAN

# guard,<files>,<ERE>,<why>[,<scan flags>[,<whole files>,<what they are>]]: the body of a check named
# for its target. <files> are find arguments under core/ naming the files to scan, and <scan flags> are
# extra "-v name=value" settings for GUARD_SCAN. <whole files> are find arguments under core/ naming
# files refused whatever they hold, each printed as "<rule>: <file>: <what they are>". Every check also
# refuses a symbolic link anywhere under core/, as "<rule>: <link>: a symbolic link": find does not
# follow a link and the Go tool does, so a link would carry in code no check has read. Prints
# "<rule>: ok", or every refusal in file and line order and then the rule's reason. A literal comma in
# an argument is written $(comma); the EREs hold no single quote and no backslash.
define guard
	@set -e; \
	if [ ! -d core ]; then echo "$@: core/: missing; the core is its own module there"; exit 1; fi; \
	hits=$$( \
		find core -type l -exec printf '$@: %s: a symbolic link\n' {} +; \
		$(if $(5),find core $(5) -type f -exec printf '$@: %s: $(6)\n' {} +;) \
		find core $(1) -type f -exec awk -v rule='$@' -v re='$(2)' $(4) "$$GUARD_SCAN" {} +); \
	if [ -z "$$hits" ]; then echo "$@: ok"; exit 0; fi; \
	printf '%s\n' "$$hits" | sort -t: -k2,2 -k3,3n; echo "$@: refused: $(3) (AGENTS.md, $@)"; exit 1
endef

comma := ,

# Every Go file under core/, test files and testdata included.
CORE_ALL = -name '*.go'
# The Go files under core/ that are built into the core: no _test.go file, nothing under testdata/.
CORE_SRC = -name '*.go' ! -name '*_test.go' ! -path '*/testdata/*'
# The files under core/, outside testdata/, that the Go tool builds into a package and that are not Go:
# assembly, C, C++, Objective-C, Fortran, SWIG and prebuilt objects.
NON_GO_SRC = ! -path '*/testdata/*' \( -name '*.s' -o -name '*.S' -o -name '*.sx' -o -name '*.c' -o \
	-name '*.cc' -o -name '*.cpp' -o -name '*.cxx' -o -name '*.h' -o -name '*.hh' -o -name '*.hpp' -o \
	-name '*.hxx' -o -name '*.m' -o -name '*.f' -o -name '*.F' -o -name '*.for' -o -name '*.f90' -o \
	-name '*.swig' -o -name '*.swigcxx' -o -name '*.syso' \)

# core-imports: an import path of adapters/, cmd/ or recipes/, in either quote form Go allows.
IMPORTS_RE = ["`]github[.]com/orilloai/placket/(adapters|cmd|recipes)(/[^"`]*)?["`]
# core-no-deps: a require directive, single or block.
REQUIRE_RE = ^[ \t]*require([ \t(]|$$)
# core-no-world: by import path, under any name, the packages for randomness, the network, the process
# and its files and environment, logs, and memory (crypto/rand, math/rand, net, os, syscall, io/ioutil,
# log, runtime, unsafe, and those below them); by name, the clock (time.Now and the calls that wait on
# or measure against it), os.Getenv and os.Open, and printing (fmt.Print*, print, println). On import
# lines only, cgo's "C". And every non-Go source file outside testdata/, which reaches the world in C or
# assembly where no grep of Go can see it.
WORLD_PKGS = crypto/rand|math/rand(/v2)?|net(/[^"`]*)?|os(/[^"`]*)?|syscall|io/ioutil|log(/[^"`]*)?|runtime(/[^"`]*)?|unsafe
WORLD_CALLS = time[.](Now|Since|Until|After|AfterFunc|Tick|NewTimer|NewTicker|Sleep)([^A-Za-z0-9_]|$$)|os[.]Getenv|os[.]Open|fmt[.]Print
WORLD_RE = (^|[^A-Za-z0-9_])($(WORLD_CALLS))|(^|[^A-Za-z0-9_.])print(ln)?[ \t]*[(]|["`]($(WORLD_PKGS))["`]
WORLD_IMPORT_RE = ["`]C["`]
# core-no-panic: panic( as a word.
PANIC_RE = (^|[^A-Za-z0-9_])panic[ \t]*[(]

core-imports:
	$(call guard,$(CORE_ALL),$(IMPORTS_RE),nothing under core/ imports adapters/$(comma) cmd/ or recipes/$(comma) and no import path is written with an escape,-v esc=1)

core-no-deps:
	@if [ ! -f core/go.mod ]; then echo "$@: core/go.mod: missing; the core is its own module"; exit 1; fi
	$(call guard,-path core/go.mod,$(REQUIRE_RE),core/go.mod requires no module)

core-no-world:
	$(call guard,$(CORE_SRC),$(WORLD_RE),no non-test code in core/ reads the clock$(comma) randomness$(comma) the environment or files$(comma) or touches the network$(comma) the process or memory$(comma) or prints; the core is Go alone$(comma) without cgo or other sources$(comma) and no import path is written with an escape,-v esc=1 -v ire='$(WORLD_IMPORT_RE)',$(NON_GO_SRC),a source file that is not Go)

core-no-panic:
	$(call guard,$(CORE_SRC),$(PANIC_RE),no non-test code in core/ calls panic)

# Every Go module's format, vet and tests. A module under a testdata/ directory is a fixture and is
# skipped. Every other module's Go files are format-checked first, whatever their build constraints,
# since the guardrail checks read gofmt form. Then a module with no Go packages on this host is skipped
# (go vet and go test refuse an empty pattern). A module whose packages cannot be listed fails here; it
# is never taken for an empty one.
modules:
	@set -e; for m in $$(find . -name go.mod -not -path './.git/*' -not -path '*/testdata/*' -exec dirname {} \; | sort); do \
		pkgs=$$(cd "$$m" && go list ./...) || { echo "== $$m: go list failed"; exit 1; }; \
		unformatted=$$(cd "$$m" && find . -name '*.go' -not -path '*/testdata/*' -exec gofmt -l {} +); \
		if [ -n "$$unformatted" ]; then echo "gofmt: $$m: not formatted:" $$unformatted; exit 1; fi; \
		if [ -z "$$pkgs" ]; then echo "== $$m: no Go packages; skipped"; continue; fi; \
		echo "== $$m"; \
		(cd "$$m" && go vet ./... && go test -count=1 ./...); done

# The repository's own shell tests.
scripts:
	@set -e; for t in tests/test_*.sh; do [ -e "$$t" ] || continue; echo "== $$t"; sh "$$t"; done
