#!/bin/sh
# The docs say what the code does, and a new user can run what they show. The checks of the docs that
# no other test holds:
# - every command in a code block of README.md and docs/guides/*.md that starts a window names every
#   flag windowCommand requires, as adapters/cli/window.go reads them; `placket window close` and
#   prose are not checked;
# - the README's judge block, its card line and its window line, takes the same flags as
#   patterns/shadow-judge/pattern.sh, which tests/test_patterns.sh runs with no key;
# - no Markdown file types a value into PLACKET_JEV_KEY on a command line;
# - the approve and merge guides and CHANGELOG.md describe the applier's fetch line as ApplierRef
#   gives it: the tag's commit, else the tag itself, which can be moved;
# - patterns/github-merge/custody.md: the agent sits only on the second ruleset's bypass list, the
#   first ruleset restricts creations, one applier serves one base branch, the forced command is
#   restricted, the check runs on a pull request made for it and covers the Actions secrets, writing
#   only a secret named at random that it deletes if the write passes, and step 1 says what is known
#   of the Workflows permission;
# - each guide's "N words" heading counts the words it defines in bold;
# - the handoff guide gives each Accord no mediator key of its own;
# - docs/release.md's example module line shows the decision module's KernelVersion;
# - the README's view paragraph and the threat model's P24 say that handing over a view discloses
#   its seal salt;
# - the threat model's judge provider row names the model, the key and the fixed words a request
#   carries beside the part's kind, body and instruction;
# - no Markdown file names a hosted placket service or a roadmap for one, and a line planted in a copy
#   of the tree is refused.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0

# fail <what> [<detail>...]: a check failed, with a detail line per argument.
fail() {
	echo "FAIL $1"
	shift
	for l; do printf '%s\n' "$l" | sed 's/^/    | /'; done
	failed=1
}
ok() { echo "ok   $1"; }

# commands <file>...: every command inside a code block of each file, one per line, with its
# continuation lines joined, as "<file>:<line>: <command>".
commands() {
	awk '
		FNR == 1 { fence = 0; cont = 0 }
		/^[ \t]*```/ { fence = !fence; cont = 0; next }
		!fence { next }
		{
			if (cont) { cmd = cmd " " $0 } else { cmd = $0; start = FNR }
			if (cmd ~ /\\$/) { sub(/\\$/, "", cmd); cont = 1; next }
			cont = 0
			print FILENAME ":" start ": " cmd
		}
	' "$@"
}

# joined <file>: every line of a shell script, with its continuation lines joined.
joined() {
	awk '{ if (cont) { cmd = cmd " " $0 } else { cmd = $0 }
		if (cmd ~ /\\$/) { sub(/\\$/, "", cmd); cont = 1; next }
		cont = 0; print cmd }' "$1"
}

# flags: the long flags of the lines on standard input, sorted, once each, on one line.
flags() { grep -oE -- '--[a-z][a-z-]*' | sort -u | tr '\n' ' '; }

# section <file> <heading>: the lines of the section under the heading "<heading>", to the next
# heading of its level or above outside a code block.
section() {
	awk -v h="$2" '
		/^```/ { fence = !fence }
		f && !fence && /^#+ / { n = index($0, " ") - 1; if (n <= level) exit }
		f { print }
		$0 == h { f = 1; level = index($0, " ") - 1 }
	' "$1"
}

cd "$root" || exit 1

# R1: a window's start names every flag windowCommand requires.
required=$(awk '/^func windowCommand/ { f = 1 } f && /r\.parse\(fs, args,/ { print; exit }' adapters/cli/window.go |
	sed 's/^.*r\.parse(fs, args, *//; s/).*$//' | grep -oE '"[a-z-]+"' | tr -d '"')
if [ -z "$required" ]; then
	fail "the flags windowCommand requires could not be read from adapters/cli/window.go"
else
	commands README.md docs/guides/*.md | grep -E '(^|[^a-z-])placket window( |$)' | grep -vE 'placket window close( |$)' >"$tmp/windows"
	: >"$tmp/missing"
	while IFS= read -r line; do
		for f in $required; do
			printf '%s\n' "$line" | grep -qE -- "--$f( |=|\$)" || echo "${line%%: *}: no --$f: ${line#*: }" >>"$tmp/missing"
		done
	done <"$tmp/windows"
	if [ ! -s "$tmp/windows" ]; then
		fail "no code block starts a window, so the check of its flags reads nothing"
	elif [ -s "$tmp/missing" ]; then
		fail "a code block starts a window without a flag windowCommand requires ($(echo $required | sed 's/\([a-z-]*\)/--\1/g'))" "$(cat "$tmp/missing")"
	else
		ok "every window a code block starts names $(echo $required | sed 's/\([a-z-]*\)/--\1/g')"
	fi
fi

# R2: the README's judge block takes the shadow-judge pattern's flags.
section README.md '## Judges' >"$tmp/judges.md"
commands "$tmp/judges.md" | sed 's/^[^ ]*: //' >"$tmp/judges"
joined patterns/shadow-judge/pattern.sh >"$tmp/pattern"
for c in card window; do
	readme=$(grep -E "(^|[^a-z-])placket $c( |\$)" "$tmp/judges" | flags)
	pattern=$(grep -E "^[ 	]*placket $c --" "$tmp/pattern" | grep -e '--judge' | flags)
	if [ -z "$pattern" ]; then
		fail "patterns/shadow-judge/pattern.sh has no placket $c line with --judge"
	elif [ "$readme" != "$pattern" ]; then
		fail "the README's judge block runs placket $c with other flags than patterns/shadow-judge/pattern.sh" \
			"README.md: $readme" "pattern.sh: $pattern"
	else
		ok "the README's judge block runs placket $c as the shadow-judge pattern does: $readme"
	fi
done

# R3: no Markdown file types the judges' key.
find . -name '*.md' -not -path './.git/*' -exec grep -nHE 'PLACKET_JEV_KEY=[^[:space:]`]' {} + >"$tmp/typed"
if [ -s "$tmp/typed" ]; then
	fail "a Markdown file types a value into PLACKET_JEV_KEY, which puts the key in shell history" "$(cat "$tmp/typed")"
else
	ok "no Markdown file types a value into PLACKET_JEV_KEY"
fi

# R11: the fetch line takes the tag's commit, else the tag itself, which can be moved.
for f in CHANGELOG.md docs/guides/approve.md docs/guides/merge.md; do
	if grep 'the tag itself' "$f" | grep -q 'can be moved'; then
		ok "$f: the fetch line falls back to the tag itself, which can be moved"
	else
		fail "$f: no line says the fetch line falls back to the tag itself, which can be moved"
	fi
done

# R12 to R17: the custody checklist.
custody=patterns/github-merge/custody.md
step() { grep -E "^$1\. \*\*" "$custody"; }
two=$(step 2)
if ! printf '%s\n' "$two" | grep -q "only on the second ruleset's bypass list"; then
	fail "$custody: step 2 does not say the agent may sit only on the second ruleset's bypass list"
elif printf '%s\n' "$two" | sed 's/no bypass of the first ruleset//g' | grep -q 'no bypass'; then
	fail "$custody: step 2 says the agent has no bypass without naming which ruleset"
else
	ok "$custody: the agent sits only on the second ruleset's bypass list"
fi
first=${two%%second ruleset*}
if printf '%s\n' "$first" | grep -q '"Restrict creations"'; then
	ok "$custody: the first ruleset restricts creations"
else
	fail "$custody: the first ruleset of step 2 has no \"Restrict creations\""
fi
if printf '%s\n' "$two" | grep -q 'one base branch' && printf '%s\n' "$two" | grep -q 'L36'; then
	ok "$custody: one applier serves one base branch (L36)"
else
	fail "$custody: step 2 does not say that one applier identity serves one base branch, with L36"
fi
if step 3 | grep -qF 'restrict,command="placket mcp --label agent"'; then
	ok "$custody: the forced command is restricted"
else
	fail "$custody: step 3's authorized_keys line is not restrict,command=\"placket mcp --label agent\""
fi
if step 6 | grep -q 'pull request made for the check' && step 6 | grep -q 'Actions secrets'; then
	ok "$custody: the check merges a pull request made for it, and covers the Actions secrets"
else
	fail "$custody: step 6 does not try the merge on a pull request made for the check, or check the Actions secrets"
fi
# The Actions secrets command of step 6 is run, with a stand-in openssl and gh first on PATH: when
# openssl fails or gives too few digits, gh is never called; when it gives 32 hex digits, gh sets a
# secret named PLACKET_CUSTODY_CHECK_ and those digits, and deletes that same name only if the set
# passed.
step 6 | grep -o '`[^`]*gh secret set[^`]*`' | tr -d '`' >"$tmp/secrets.sh"
mkdir "$tmp/stand-in"
cat >"$tmp/stand-in/openssl" <<'STAND_IN'
#!/bin/sh
case $OPENSSL_GIVES in
fail) printf 'ab'; exit 1 ;;
nothing) exit 0 ;;
short) echo 0123abc ;;
hex) echo 0123456789abcdef0123456789abcdef ;;
esac
STAND_IN
cat >"$tmp/stand-in/gh" <<'STAND_IN'
#!/bin/sh
echo "$*" >>"$GH_LOG"
[ "$1 $2" != "secret set" ] || exit "$GH_SET_EXIT"
STAND_IN
chmod +x "$tmp/stand-in/openssl" "$tmp/stand-in/gh"
# secrets <openssl gives> <set exit>: runs the command, and prints what gh was called with.
secrets() {
	: >"$tmp/gh.log"
	(cd "$tmp" && OPENSSL_GIVES=$1 GH_SET_EXIT=$2 GH_LOG="$tmp/gh.log" PATH="$tmp/stand-in:$PATH" \
		sh "$tmp/secrets.sh" >/dev/null 2>&1)
	cat "$tmp/gh.log"
}
name=PLACKET_CUSTODY_CHECK_0123456789abcdef0123456789abcdef
if [ "$(wc -l <"$tmp/secrets.sh" | tr -d ' ')" != 1 ]; then
	fail "$custody: step 6 has not one gh secret set command"
else
	bad=
	for gives in fail nothing short; do
		called=$(secrets "$gives" 1)
		[ -z "$called" ] || bad="$bad${bad:+; }with openssl giving $gives, gh was called: $called"
	done
	called=$(secrets hex 1)
	[ "$called" = "secret set $name --repo OWNER/NAME --body x" ] ||
		bad="$bad${bad:+; }with the set refused, gh was called: $called"
	called=$(secrets hex 0 | tr '\n' '|')
	[ "$called" = "secret set $name --repo OWNER/NAME --body x|secret delete $name --repo OWNER/NAME|" ] ||
		bad="$bad${bad:+; }with the set passed, gh was called: $called"
	if [ -n "$bad" ]; then
		fail "$custody: step 6's Actions secrets command does not set only a secret named at random, deleted if the set passes" "$bad"
	else
		ok "$custody: the Actions secrets check sets only a secret named at random, and deletes it if the set passes"
	fi
fi
one=$(step 1)
if ! printf '%s\n' "$one" | grep -q 'Workflows'; then
	fail "$custody: step 1 names no Workflows permission"
elif printf '%s\n' "$one" | grep -q 'not known'; then
	if printf '%s\n' "$one" | grep -q 'https\?://'; then
		fail "$custody: step 1 says the Workflows permission is not known, and links a page"
	else
		ok "$custody: step 1 says what is not known of the Workflows permission"
	fi
elif printf '%s\n' "$one" | grep -q 'https://docs\.github\.com/'; then
	ok "$custody: step 1 states the Workflows permission from GitHub's docs"
else
	fail "$custody: step 1 names the Workflows permission with neither GitHub's docs nor \"not known\""
fi

# R18: each guide's "N words" heading counts its bold words.
for f in docs/guides/*.md; do
	h=$(grep -E '^## [A-Z][a-z]+ words$' "$f") || continue
	word=$(echo "$h" | sed 's/^## \([A-Za-z]*\) words$/\1/' | tr 'A-Z' 'a-z')
	n=$(echo "one two three four five six seven eight nine ten" | tr ' ' '\n' | grep -nx "$word" | cut -d: -f1)
	got=$(section "$f" "$h" | grep -oE '\*\*[^*]+\*\*' | wc -l | tr -d ' ')
	if [ -z "$n" ]; then
		fail "$f: \"$h\" is no number from one to ten"
	elif [ "$n" != "$got" ]; then
		fail "$f: \"$h\" defines $got words in bold, not $n"
	else
		ok "$f: \"$h\" defines $n words"
	fi
done

# R19: the handoff guide's two Accords share one mediator key.
grep -nE "own window, journal and keys|mediator key of its own|its own mediator key|one mediator's receipts" docs/guides/handoff.md >"$tmp/own"
if [ -s "$tmp/own" ]; then
	fail "docs/guides/handoff.md gives each Accord a mediator key of its own; placket init handoff writes one for both" "$(cat "$tmp/own")"
elif ! grep -q 'one mediator key' docs/guides/handoff.md; then
	fail "docs/guides/handoff.md does not say that one mediator key serves both Accords"
else
	ok "docs/guides/handoff.md: one mediator key for both Accords"
fi

# R20: the release doc's module line is the decision module's version.
kernel=$(sed -n 's/^const KernelVersion = "\(.*\)"$/\1/p' core/composition/kernel.go)
shown=$(sed -n 's/^module \([^ ]*\) .*$/\1/p' docs/release.md)
if [ -z "$kernel" ]; then
	fail "no KernelVersion in core/composition/kernel.go"
elif [ "$shown" != "$kernel" ]; then
	fail "docs/release.md shows module $shown, not the KernelVersion $kernel"
else
	ok "docs/release.md shows module $kernel, the KernelVersion"
fi

# R23: a view handed over discloses its seal salt.
view=$(awk '/^A party also checks its own receipts against the view/ { f = 1 } f && /^## / { exit } f { print }' README.md)
if printf '%s\n' "$view" | grep -q 'discloses its seal salt'; then
	ok "README.md: a view handed over discloses its seal salt"
else
	fail "README.md: the view paragraph does not say that handing over a view discloses its seal salt"
fi
if grep -E '^- \*\*P24\.' docs/security/threat-model.md | grep -q 'discloses its seal salt'; then
	ok "docs/security/threat-model.md: P24 says a view handed over discloses its seal salt"
else
	fail "docs/security/threat-model.md: P24 does not say that handing over a view discloses its seal salt"
fi

# The threat model's summary of what a judge provider is sent names, beside the part's kind, body and
# instruction, the rest of the request: the config's model, the key and the adapter's fixed words.
row=$(grep '^| Judge provider |' docs/security/threat-model.md)
if printf '%s\n' "$row" | grep -q "model" && printf '%s\n' "$row" | grep -q 'the key' && printf '%s\n' "$row" | grep -q 'fixed words'; then
	ok "docs/security/threat-model.md: the judge provider's row names the model, the key and the fixed words sent"
else
	fail "docs/security/threat-model.md: the judge provider's row does not name the model, the key and the fixed words a request carries"
fi

# R24: no Markdown file names a hosted placket service or a roadmap. hosted <dir> prints each line
# under <dir> that says "hosted", other than "self-hosted", or "roadmap".
hosted() {
	find "$1" -name '*.md' -not -path '*/.git/*' -exec awk '
		{ l = tolower($0); gsub(/self-hosted/, "", l) }
		l ~ /hosted|roadmap/ { print FILENAME ":" FNR ": " $0 }
	' {} +
}
hosted . >"$tmp/hosted"
if [ -s "$tmp/hosted" ]; then
	fail "a Markdown file names a hosted service or a roadmap" "$(cat "$tmp/hosted")"
else
	ok "no Markdown file names a hosted service or a roadmap"
fi
mkdir "$tmp/tree"
find . -name '*.md' -not -path './.git/*' | while IFS= read -r f; do
	mkdir -p "$tmp/tree/$(dirname "$f")"
	cp "$f" "$tmp/tree/$f"
done
echo 'A Hosted placket is on the way.' >>"$tmp/tree/docs/release.md"
echo 'Runners stay self-hosted.' >>"$tmp/tree/README.md"
hosted "$tmp/tree" | sed "s|^$tmp/tree/||" >"$tmp/planted"
if [ "$(cat "$tmp/planted")" = "docs/release.md:$(wc -l <"$tmp/tree/docs/release.md" | tr -d ' '): A Hosted placket is on the way." ]; then
	ok "a line planted in a copy of the tree that names a hosted service is refused, and self-hosted is not"
else
	fail "the planted line was not the one line refused" "$(cat "$tmp/planted")"
fi

exit $failed
