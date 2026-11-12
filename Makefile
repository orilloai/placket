# `make ci` is placket's one entry point for checks; GitHub runs it on every push and pull request.
#
# The guardrail checks run first, each as its own step named for the rule it enforces (AGENTS.md).
# Every line a check prints starts with the rule's name: "<rule>: ok" when it passes, and
# "<rule>: <file>:<line>: <code>" for each offending line when it refuses. Then every Go module's
# format, vet and tests, and every tests/test_*.sh. The protocol's checks, specs-name-behaviour and
# schemas-hold, read spec/ and print "<rule>: <file>[:<line>]: ..." in the same way.
.PHONY: ci core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold modules scripts
ci: core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold modules scripts

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

# SPECS_SCAN is an awk program, run over spec/reserved.txt, spec/ids.txt and spec/protocol.md in that
# order. It prints "specs-name-behaviour: <file>:<line>: <id>: <what is wrong>" for each fault: an id
# list line that is not one id, or an id listed twice or in both lists; a level-3 heading of the text
# that is not "### <id> <title>"; a reserved id as a heading; an id that heads two requirements; a
# heading id not in spec/ids.txt, or a listed id with no heading; a requirement with no "#### "
# scenario before the next heading of level 1 to 3; and any line of the text that names Go code: a
# path under core/, adapters/ or cmd/, the word "func " or a package-qualified name such as x.Name.
define SPECS_SCAN
function idok(s) { return s ~ /^[a-z]+(-[a-z]+)*-[1-9][0-9]*$$/ }
function bad(f, n, m) { print "specs-name-behaviour: " f ":" n ": " m }
function close_req() { if (cur != "" && !scen) bad(F, curl, cur ": no scenario (a \"#### \" heading under it)"); cur = "" }
FILENAME != last { last = FILENAME; file++ }
file == 1 || file == 2 {
	w = (file == 1) ? "reserved" : "listed"
	if (!idok($$0)) { bad(FILENAME, FNR, "not one requirement id per line: " $$0); next }
	if (file == 1) { if ($$0 in res) { bad(FILENAME, FNR, $$0 ": listed twice"); next } res[$$0] = FNR; next }
	if ($$0 in ids) { bad(FILENAME, FNR, $$0 ": listed twice"); next }
	if ($$0 in res) { bad(FILENAME, FNR, $$0 ": also in spec/reserved.txt"); next }
	ids[$$0] = FNR; order[++nids] = $$0; next
}
{
	F = FILENAME
	if ($$0 ~ /^#### /) scen = 1
	if ($$0 ~ /^#(#(#)?)? /) {
		close_req()
		if ($$0 ~ /^### /) {
			id = $$2; title = $$0; sub(/^### [^ ]+ */, "", title)
			if (!idok(id) || title == "") { bad(F, FNR, "a level-3 heading that is not \"### <id> <title>\": " $$0) }
			else if (id in res) { bad(F, FNR, id ": a reserved id (spec/reserved.txt)") }
			else if (id in seen) { bad(F, FNR, id ": a second heading for this id (the first is at line " seen[id] ")") }
			else {
				seen[id] = FNR
				if (!(id in ids)) bad(F, FNR, id ": not listed in spec/ids.txt")
				cur = id; curl = FNR; scen = 0
			}
		}
	}
	if ($$0 ~ /core\/|adapters\/|cmd\/|func |[a-z][.][A-Z]/) bad(F, FNR, "names Go code: " $$0)
}
END {
	close_req()
	for (i = 1; i <= nids; i++) if (!(order[i] in seen)) bad("spec/ids.txt", ids[order[i]], order[i] ": no requirement heading in spec/protocol.md")
}
endef
export SPECS_SCAN

specs-name-behaviour:
	@set -e; for f in spec/protocol.md spec/ids.txt spec/reserved.txt; do \
		if [ ! -f "$$f" ]; then echo "$@: $$f: missing"; exit 1; fi; done; \
	hits=$$(awk "$$SPECS_SCAN" spec/reserved.txt spec/ids.txt spec/protocol.md); \
	if [ -z "$$hits" ]; then echo "$@: ok"; exit 0; fi; \
	printf '%s\n' "$$hits"; echo "$@: refused: the requirement text names behaviour under the ids spec/ids.txt lists, each once with a scenario, and never Go code (AGENTS.md, $@)"; exit 1

# schemas-hold: every schema in spec/schema/ is JSON, declares JSON Schema draft 2020-12, has the $id
# its file name gives under SCHEMA_URL, refers to nothing outside that prefix, is named by a row of
# spec/frozen-formats.md, has accepted and refused examples under spec/schema/testdata/<name>/, and
# compiles; every accepted example validates and every refused one is refused. The validator is the
# jv command at the exact tag JV names. It is fetched by `go run` once, then run with the Go module proxy pointed
# at the local module cache and every HTTP proxy at a closed port, so a schema can load nothing from
# the network: the --map option serves SCHEMA_URL from spec/schema/. All examples go through one run
# of jv, each wrapped as {"schema": <name>, "value": <example>} and checked by a schema that sends
# each to its own; a schema that fails to compile there is found by compiling each one alone. jq reads
# the schemas' own members. Example files are named in lowercase words joined by hyphens.
JV = github.com/santhosh-tekuri/jsonschema/cmd/jv@v0.7.0
SCHEMA_URL = https://placket.dev/schema/0.1/
define SCHEMAS_HOLD
set -u
rule=schemas-hold; S=spec/schema; T=$$S/testdata; R=spec/frozen-formats.md; draft=https://json-schema.org/draft/2020-12/schema
say() { echo "$$rule: $$*"; }
bad=0
refuse() { say "$$@"; bad=1; }
if [ ! -d "$$S" ]; then say "$$S/: missing"; exit 1; fi
if [ ! -f "$$R" ]; then say "$$R: missing; every schema needs a row of the register"; exit 1; fi
command -v jq >/dev/null || { say "jq: not found; this check needs it"; exit 1; }
tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; : >"$$tmp/notjson"
go run "$(JV)" --version >"$$tmp/fetch" 2>&1 || { say "go run $(JV): failed"; sed 's/^/    /' "$$tmp/fetch"; exit 1; }
offline() { HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 NO_PROXY= no_proxy= http_proxy= https_proxy= \
	GOPROXY="file://$$(go env GOMODCACHE)/cache/download" GOFLAGS=-mod=mod go run "$(JV)" "$$@" 2>&1; }
names=
for f in "$$S"/*.json; do
	[ -e "$$f" ] || continue
	n=$${f##*/}; n=$${n%.json}
	if ! jq empty "$$f" >/dev/null 2>&1; then refuse "$$f: not JSON"; continue; fi
	if [ "$$(jq -r 'if type == "object" then (."$$schema" // "") else "" end | tostring' "$$f")" != "$$draft" ]; then
		refuse "$$f: not a JSON Schema draft 2020-12 schema: its \$$schema is not $$draft"; continue; fi
	if [ "$$(jq -r '."$$id" // "" | tostring' "$$f")" != "$(SCHEMA_URL)$$n.json" ]; then
		refuse "$$f: its \$$id is not $(SCHEMA_URL)$$n.json"; continue; fi
	out=$$(jq -r --arg u "$(SCHEMA_URL)" --arg d "$$draft" '[.. | objects | to_entries[] | select(.key == "$$ref" or .key == "$$dynamicRef" or .key == "$$schema" or .key == "$$id") | select(.value | (type != "string") or (test("^[a-z][a-z0-9+.-]*:") and ((startswith($$u) or . == $$d) | not))) | .value | tostring] | .[0] // empty' "$$f")
	if [ -n "$$out" ]; then refuse "$$f: names a schema outside $(SCHEMA_URL): $$out"; continue; fi
	if ! grep -qF "| \`$$f\`" "$$R" && ! grep -qF ", \`$$f\`" "$$R"; then refuse "$$f: no row of $$R names it"; continue; fi
	for v in accepted refused; do
		set -- "$$T/$$n/$$v"/*.json
		if [ ! -e "$$1" ]; then refuse "$$f: no $$v example in $$T/$$n/$$v/"; continue 2; fi
	done
	names="$$names $$n"
done
for d in "$$T"/*; do
	[ -e "$$d" ] || continue
	n=$${d##*/}
	if [ ! -f "$$S/$$n.json" ]; then refuse "$$d/: examples of no schema; there is no $$S/$$n.json"; continue; fi
	for e in "$$d"/* "$$d"/*/*; do
		case "$$e" in "$$d/accepted" | "$$d/refused" | "$$d"/accepted/*.json | "$$d"/refused/*.json) ;; *"*") ;;
		*) refuse "$$e: not an example; examples are $$d/accepted/*.json and $$d/refused/*.json" ;; esac
	done
	files=
	for e in "$$d"/accepted/*.json "$$d"/refused/*.json; do [ -e "$$e" ] && files="$$files $$e"; done
	[ -n "$$files" ] && ! jq -n 'inputs' $$files >/dev/null 2>&1 || continue
	for e in $$files; do
		jq empty "$$e" >/dev/null 2>&1 || { refuse "$$e: not JSON"; echo "$$e" >>"$$tmp/notjson"; }
	done
done
dispatch() {
	{ printf '{"$$schema":"%s","type":"object","required":["schema","value"],"allOf":[' "$$draft"; sep=
	  for n in $$1; do printf '%s{"if":{"properties":{"schema":{"const":"%s"}}},"then":{"properties":{"value":{"$$ref":"%s%s.json"}}}}' "$$sep" "$$n" "$(SCHEMA_URL)" "$$n"; sep=,; done
	  printf ']}'; } >"$$tmp/dispatch.json"
}
dispatch "$$names"
if ! offline -q --map "$(SCHEMA_URL)=$$PWD/$$S" "$$tmp/dispatch.json" >"$$tmp/compile"; then
	ok=
	for n in $$names; do
		if offline --map "$(SCHEMA_URL)=$$PWD/$$S" "$(SCHEMA_URL)$$n.json" >"$$tmp/one"; then ok="$$ok $$n"
		else refuse "$$S/$$n.json: does not compile as JSON Schema draft 2020-12 with nothing fetched"; sed -n '2,6s/^/    /p' "$$tmp/one"; fi
	done
	names=$$ok; dispatch "$$names"
fi
i=0; : >"$$tmp/list"; mkdir "$$tmp/i"
for n in $$names; do
	for v in accepted refused; do
		for e in "$$T/$$n/$$v"/*.json; do
			[ -e "$$e" ] && ! grep -qxF "$$e" "$$tmp/notjson" || continue
			i=$$((i + 1)); { printf '{"schema":"%s","value":' "$$n"; cat "$$e"; printf '}\n'; } >"$$tmp/i/$$i.json"
			echo "$$i $$n $$v $$e" >>"$$tmp/list"
		done
	done
done
if [ -n "$$names" ]; then
	offline --map "$(SCHEMA_URL)=$$PWD/$$S" "$$tmp/dispatch.json" $$(awk '{ print "'"$$tmp"'/i/" $$1 ".json" }' "$$tmp/list") >"$$tmp/run" || :
	if ! grep -q '^schema .*: ok$$' "$$tmp/run"; then refuse "$$S/: the schemas do not compile together"; sed -n '1,8s/^/    /p' "$$tmp/run"; exit 1; fi
	awk -v t="$$tmp/i/" -v rule="$$rule" -v names="$$names" '
		FNR == NR { n[$$1] = $$2; v[$$1] = $$3; e[$$1] = $$4; next }
		/^instance / { k = $$2; sub(/:$$/, "", k); sub("^" t, "", k); sub(/[.]json$$/, "", k); got = $$3; inst = k
			if (v[k] == "accepted") { a[n[k]]++; if (got != "ok") { print rule ": " e[k] ": refused by spec/schema/" n[k] ".json"; badn[n[k]] = 1 } }
			else { r[n[k]]++; if (got != "failed") { print rule ": " e[k] ": accepted by spec/schema/" n[k] ".json"; badn[n[k]] = 1 } }
			next }
		inst != "" && v[inst] == "accepted" && /^[ \t]*- / && shown[inst]++ < 3 { print "    " $$0 }
		END { c = split(names, s, " "); for (j = 1; j <= c; j++) if (!(s[j] in badn)) print rule ": spec/schema/" s[j] ".json: ok, " a[s[j]] + 0 " accepted, " r[s[j]] + 0 " refused" }
	' "$$tmp/list" "$$tmp/run" >"$$tmp/verdicts"
	cat "$$tmp/verdicts"
	grep -q '^schemas-hold: .*: \(refused\|accepted\) by spec/schema/' "$$tmp/verdicts" && bad=1
fi
if [ "$$bad" -ne 0 ]; then say "refused: every schema compiles from the tree with nothing fetched, holds against its examples and has a row in the register (AGENTS.md, $$rule)"; exit 1; fi
say ok
endef
export SCHEMAS_HOLD

schemas-hold:
	@sh -c "$$SCHEMAS_HOLD"

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
