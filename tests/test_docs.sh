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
#   restricted, the check runs on a pull request made for it and covers the Actions secrets, and step
#   1 says what is known of the Workflows permission;
# - each guide's "N words" heading counts the words it defines in bold;
# - the handoff guide gives each Accord no mediator key of its own;
# - docs/release.md's example module line shows the decision module's KernelVersion;
# - the README's view paragraph and the threat model's P24 say that handing over a view discloses
#   its seal salt;
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
