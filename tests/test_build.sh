#!/bin/sh
# The test of what make ci compiles once and keeps: build compiles the standard library's packages the
# modules import, both ways, before any step that compiles, and lint keeps the linter's cache inside
# Go's build cache.
#
# Each case makes a minimal copy: the Makefile and the two modules of tests/testdata/parallel/modules/,
# whose test files are kept as <name>.in and named at setup. It runs make with a go on PATH that
# writes down each go command and then runs the real one.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
fixtures=$root/tests/testdata/parallel
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0
out=$tmp/out

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$out"
	failed=$((failed + 1))
}

# copy <dir>: the Makefile and the fixture's files at <dir> in $tmp/repo, each <name>.in named <name>.
copy() {
	rm -rf "$tmp/repo"
	mkdir -p "$tmp/repo/$1"
	cp "$root/Makefile" "$tmp/repo/"
	cp -R "$fixtures/$2/." "$tmp/repo/$1/"
	find "$tmp/repo" -name '*.in' | while read -r f; do mv "$f" "${f%.in}"; done
}

# shown <line>...: the output holds each line given, in that order, with lines in between allowed.
shown() {
	printf '%s\n' "$@" >"$tmp/want"
	awk 'NR == FNR { w[++n] = $0; next } i < n && $0 == w[i + 1] { i++ } END { exit i != n }' "$tmp/want" "$out"
}

# The cases below run make with a go on PATH that writes one line per command to $golog, the linter's
# cache and the arguments, "<STATICCHECK_CACHE>|<arguments>", and then runs the real go.
realgo=$(command -v go)
golog=$tmp/go.log
mkdir "$tmp/bin"
printf '#!/bin/sh\nprintf "%%s|%%s\\n" "${STATICCHECK_CACHE-unset}" "$*" >>"%s"\nexec "%s" "$@"\n' "$golog" "$realgo" >"$tmp/bin/go"
chmod +x "$tmp/bin/go"

# build: a step that compiles (here vet) first has the standard library's packages its modules import
# compiled once, as they are and with the race detector, and only then runs.
name="build compiles the standard library first"
cases=$((cases + 1))
copy . modules
: >"$golog"
(cd "$tmp/repo" && PATH=$tmp/bin:$PATH MAKEFLAGS= MAKELEVEL= make -k --no-print-directory vet) >"$out" 2>&1
rc=$?
order=$(awk -F'|' '$2 ~ /^build -race / { print "race"; next } $2 ~ /^build / { print "plain"; next } $2 ~ /^vet / { print "vet" }' "$golog" | uniq | paste -sd' ' -)
if [ "$rc" -ne 0 ]; then bad "make failed; vet should pass"
elif ! awk -F'|' '$2 ~ /^build (-race )?/ && $2 ~ / testing( |$)/ { n++ } END { exit n != 2 }' "$golog"; then
	bad "the standard library's testing was not compiled both ways: $(cat "$golog")"
elif [ "$order" != "plain race vet" ]; then bad "the go commands ran in the order: $order"
elif ! shown 'build: ok' 'vet: ok'; then bad "build did not end before vet"
elif ! grep -q '^time: build: [0-9][0-9]*s$' "$out"; then bad "no time line for build"
else echo "ok   $name: plain, then with the race detector, then vet"; fi

# lint: the linter's cache is kept inside Go's build cache, unless STATICCHECK_CACHE is set.
name="lint keeps its cache in the build cache"
cases=$((cases + 1))
copy . modules
gocache=$("$realgo" env GOCACHE)
: >"$golog"
(unset STATICCHECK_CACHE; cd "$tmp/repo" && PATH=$tmp/bin:$PATH MAKEFLAGS= MAKELEVEL= make -k --no-print-directory lint) >"$out" 2>&1
rc=$?
caches=$(awk -F'|' '$2 ~ /^run honnef[.]co\/go\/tools\/cmd\/staticcheck@/ { print $1 }' "$golog" | sort -u)
if [ "$rc" -ne 0 ]; then bad "make failed; lint should pass"
elif [ "$caches" != "$gocache/staticcheck" ]; then bad "the linter's cache was '$caches', not $gocache/staticcheck"
else
	: >"$golog"
	(cd "$tmp/repo" && STATICCHECK_CACHE=$tmp/sc PATH=$tmp/bin:$PATH MAKEFLAGS= MAKELEVEL= make -k --no-print-directory lint) >"$out" 2>&1
	rc=$?
	caches=$(awk -F'|' '$2 ~ /^run honnef[.]co\/go\/tools\/cmd\/staticcheck@/ { print $1 }' "$golog" | sort -u)
	if [ "$rc" -ne 0 ]; then bad "make failed with STATICCHECK_CACHE set; lint should pass"
	elif [ "$caches" != "$tmp/sc" ]; then bad "the linter's cache was '$caches', not the STATICCHECK_CACHE given"
	else echo "ok   $name: $gocache/staticcheck, or the one given"; fi
fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
