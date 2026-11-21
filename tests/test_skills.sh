#!/bin/sh
# The skills under skills/ and the Claude Code plugin that bundles them (skills/README.md):
# - each skills/<name>/SKILL.md opens with front matter of exactly a name, its directory's, and a
#   description, and every `placket <command>` its code names is a command `placket --help` lists, as
#   the placket built from the tree prints it; a SKILL.md that names another command is refused;
# - skills/placket-setup/fetch-pattern.sh copies a pattern from the repository at a release's tag, or
#   at main for a version that is no tag, and says so; with --list, it prints the patterns listed at
#   that ref; it refuses a pattern the ref lacks, a name that is not a pattern's, and a directory that
#   holds anything. PLACKET_REPO points it at a stand-in repository built here, whose v0.1.0 tag and
#   main differ;
# - skills/placket-setup/SKILL.md names only patterns the tree holds, and names local-apply and
#   github-merge as the patterns placket init approve and merge take their appliers from;
# - .claude-plugin/marketplace.json and plugin.json parse and carry what Claude Code reads: one
#   plugin, placket, whose source is the repository root, and a plugin.json with no version, no skill
#   list and no MCP server, beside both skills.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
out=$tmp/out

fail() {
	echo "FAIL $1"
	[ ! -s "$out" ] || sed 's/^/    | /' "$out"
	failed=1
}

(cd "$root" && go build -o "$tmp/placket" ./cmd/placket) || { echo "FAIL the tree's placket does not build"; exit 1; }
"$tmp/placket" --help | sed -n 's/^placket \([a-z-]*\) \[flags\]$/\1/p' >"$tmp/commands"
[ -s "$tmp/commands" ] || { echo "FAIL placket --help lists no commands"; exit 1; }

# check <file>: the skill <file> is well formed. It prints one line per fault, "<file>: <fault>",
# and exits 1 when there is one.
check() {
	f=$1
	d=$(basename "$(dirname "$f")")
	bad=0
	# The front matter: the first line ---, then exactly name and description, then ---.
	head -n 1 "$f" | grep -qx -- '---' || { echo "$f: no front matter"; return 1; }
	awk 'NR > 1 && /^---$/ { exit } NR > 1 { print }' "$f" >"$tmp/front"
	keys=$(sed -n 's/^\([a-z_-]*\):.*$/\1/p' "$tmp/front" | sort | tr '\n' ' ')
	if [ "$keys" != "description name " ] || [ "$(grep -c . "$tmp/front")" -ne 2 ]; then
		echo "$f: front matter holds other than one name and one description: $keys"
		bad=1
	fi
	name=$(sed -n 's/^name: *//p' "$tmp/front")
	[ "$name" = "$d" ] || { echo "$f: name '$name' is not its directory's, '$d'"; bad=1; }
	expr "$name" : '[a-z0-9][a-z0-9-]*$' >/dev/null && [ ${#name} -le 64 ] ||
		{ echo "$f: name '$name' is not 1 to 64 of a-z, 0-9 and -"; bad=1; }
	desc=$(sed -n 's/^description: *//p' "$tmp/front")
	[ -n "$desc" ] && [ ${#desc} -le 1024 ] || { echo "$f: the description is empty or longer than 1024 characters"; bad=1; }
	# The code: every line inside a fence, and every span in backquotes outside one. In it, each
	# placket followed by a word names a command.
	awk '
		/^[ \t]*```/ { fence = !fence; next }
		fence { print; next }
		{ s = $0; while (match(s, /`[^`]*`/)) { print substr(s, RSTART + 1, RLENGTH - 2); s = substr(s, RSTART + RLENGTH) } }
	' "$f" | grep -oE '(^|[ /("])placket [a-z][a-z-]*' | sed 's/^.*placket //' | sort -u >"$tmp/named"
	while read -r c; do
		grep -qx "$c" "$tmp/commands" || { echo "$f: names placket $c, which placket --help does not list"; bad=1; }
	done <"$tmp/named"
	return $bad
}

# U7: each skill, then a planted one that names a command placket lacks.
for s in placket-setup placket-scout; do
	[ -f "$root/skills/$s/SKILL.md" ] || { : >"$out"; fail "skills/$s/SKILL.md is missing"; }
done
for f in "$root"/skills/*/SKILL.md; do
	[ -e "$f" ] || continue
	if check "$f" >"$out"; then
		echo "ok   ${f#"$root"/}: front matter, and every placket command it names is one placket --help lists"
	else
		fail "${f#"$root"/} is not well formed"
	fi
done
mkdir -p "$tmp/skills/planted"
printf -- '---\nname: planted\ndescription: A planted skill.\n---\n\nRun `placket version`, then:\n\n```sh\nplacket frobnicate --label x\n```\n' >"$tmp/skills/planted/SKILL.md"
if check "$tmp/skills/planted/SKILL.md" >"$out"; then
	fail "a skill that names placket frobnicate is not refused"
elif [ "$(cat "$out")" != "$tmp/skills/planted/SKILL.md: names placket frobnicate, which placket --help does not list" ]; then
	fail "a skill that names placket frobnicate is refused for another reason"
else
	echo "ok   a skill that names a command placket --help does not list is refused"
fi
printf -- '---\nname: planted\ndescription: A planted skill.\nallowed-tools: Bash\n---\n\n`placket version`\n' >"$tmp/skills/planted/SKILL.md"
if check "$tmp/skills/planted/SKILL.md" >"$out"; then
	fail "a skill whose front matter holds a harness's own key is not refused"
else
	echo "ok   a skill whose front matter holds more than a name and a description is refused"
fi

# The setup skill calls a placket init example a pattern only where init takes it from one: every
# pattern it names, as patterns/<name>, is a directory of the tree, approve takes local-apply and
# merge github-merge, and handoff, which has none, is not counted among the patterns.
setup=$root/skills/placket-setup/SKILL.md
: >"$out"
for p in $(grep -oE 'patterns/[a-z0-9-]+' "$setup" | sed 's|^patterns/||' | sort -u); do
	[ -d "$root/patterns/$p" ] || echo "names patterns/$p, which is no pattern of the tree" >>"$out"
done
grep -qE 'placket init approve`[^`]*`patterns/local-apply/`' "$setup" ||
	echo "does not say that placket init approve takes its applier from patterns/local-apply/" >>"$out"
grep -qE 'placket init merge`[^`]*`patterns/github-merge/`' "$setup" ||
	echo "does not say that placket init merge takes its applier from patterns/github-merge/" >>"$out"
grep -n 'Three of the patterns' "$setup" >>"$out"
if [ -s "$out" ]; then
	fail "skills/placket-setup/SKILL.md calls an example a pattern that init does not take from one"
else
	echo "ok   skills/placket-setup/SKILL.md names local-apply and github-merge for approve and merge, and every pattern it names is one"
fi

# U8: fetch-pattern.sh, against a stand-in repository whose v0.1.0 tag and main differ.
fetch=$root/skills/placket-setup/fetch-pattern.sh
repo=$tmp/repo
git init -q -b main "$repo"
g() { git -C "$repo" -c user.name=test -c user.email=test@example.invalid "$@"; }
mkdir -p "$repo/patterns/local-apply"
printf '# Patterns\n\n## The patterns\n\n- `local-apply`: writes a file. Checked against placket v0.1.0.\n- `notifier`: runs a command. Checked against placket v0.1.0.\n\n## What a pattern is made of\n\n- `not-a-pattern`: a line of another section.\n' >"$repo/patterns/README.md"
echo "local-apply at v0.1.0" >"$repo/patterns/local-apply/README.md"
printf '#!/bin/sh\necho v0.1.0\n' >"$repo/patterns/local-apply/apply.sh"
chmod 755 "$repo/patterns/local-apply/apply.sh"
g add -A && g commit -q -m v0.1.0 && g tag v0.1.0
echo "local-apply at main" >"$repo/patterns/local-apply/README.md"
echo "a file added on main" >"$repo/patterns/local-apply/later.txt"
mkdir -p "$repo/patterns/new-on-main"
echo "new" >"$repo/patterns/new-on-main/README.md"
printf -- '- `new-on-main`: new.\n' >>"$repo/patterns/README.md"
g add -A && g commit -q -m main
tagged=$(g rev-parse v0.1.0)
main=$(g rev-parse main)

# run <args>...: fetch-pattern.sh with the stand-in repository, from a fresh directory; output to
# $out, status to $rc.
n=0
run() {
	n=$((n + 1))
	mkdir -p "$tmp/cwd$n"
	(cd "$tmp/cwd$n" && PLACKET_REPO=$repo sh "$fetch" "$@") >"$out" 2>&1
	rc=$?
}
# same <name> <ref> <dir>: <dir> holds exactly patterns/local-apply/ as at <ref>, modes included.
same() {
	rm -rf "$tmp/want"
	mkdir "$tmp/want"
	g archive "$2:patterns/local-apply" | tar -x -C "$tmp/want"
	if ! diff -r "$tmp/want" "$3" >"$tmp/diff" 2>&1; then
		sed 's/^/    > /' "$tmp/diff"
		fail "$1: the directory is not patterns/local-apply/ as at $2"
	elif [ ! -x "$3/apply.sh" ]; then
		fail "$1: apply.sh is not executable"
	else
		echo "ok   $1"
	fi
}

run v0.1.0 local-apply D
if [ "$rc" -ne 0 ]; then
	fail "fetch-pattern.sh v0.1.0 local-apply D exits $rc"
elif [ "$(cat "$out")" != "fetch-pattern: patterns/local-apply/ at v0.1.0, commit $tagged, into D" ]; then
	fail "fetch-pattern.sh v0.1.0 local-apply D does not say what it fetched, at which commit"
else
	same "fetch-pattern.sh v0.1.0 local-apply D copies the pattern as at the tag" v0.1.0 "$tmp/cwd$n/D"
fi

run v0.0.0-20261119142215-d1f30bf10f61 local-apply D
if [ "$rc" -ne 0 ]; then
	fail "fetch-pattern.sh of a version with no tag exits $rc"
elif [ "$(cat "$out")" != "fetch-pattern: v0.0.0-20261119142215-d1f30bf10f61 is no release tag, so this takes main, which can change at any time
fetch-pattern: patterns/local-apply/ at main, commit $main, into D" ]; then
	fail "fetch-pattern.sh of a version with no tag does not say it takes main"
else
	same "fetch-pattern.sh of a version with no tag copies the pattern as at main, and says so" main "$tmp/cwd$n/D"
fi

run --list v0.1.0
if [ "$rc" -ne 0 ] || [ "$(cat "$out")" != '- `local-apply`: writes a file. Checked against placket v0.1.0.
- `notifier`: runs a command. Checked against placket v0.1.0.' ]; then
	fail "fetch-pattern.sh --list v0.1.0 does not print the patterns listed at the tag (exit $rc)"
else
	echo "ok   fetch-pattern.sh --list v0.1.0 prints the patterns listed at the tag"
fi

# refused <name> <exit> <last line> <args>...: fetch-pattern.sh refuses, and writes no D.
refused() {
	name=$1 want=$2 last=$3
	shift 3
	run "$@"
	if [ "$rc" -ne "$want" ] || [ "$(tail -n 1 "$out")" != "$last" ]; then
		fail "$name: want exit $want and last line '$last', got exit $rc"
	elif [ -e "$tmp/cwd$n/D" ] && [ -n "$(ls -A "$tmp/cwd$n/D")" ]; then
		fail "$name: it wrote into D"
	else
		echo "ok   $name"
	fi
}
refused "a pattern the tag lacks is refused" 1 \
	"fetch-pattern: refused: new-on-main: no pattern of that name at v0.1.0" v0.1.0 new-on-main D
refused "a name that is not a pattern's is refused" 2 \
	"fetch-pattern: usage: fetch-pattern.sh <version> <pattern> <dir> | --list <version>" v0.1.0 ../local-apply D
refused "a tag the repository lacks is refused" 1 \
	"fetch-pattern: refused: v9.9.9: could not fetch the repository at that ref" v9.9.9 local-apply D
mkdir "$tmp/cwd$((n + 1))"
echo kept >"$tmp/cwd$((n + 1))/D"
run v0.1.0 local-apply D
if [ "$rc" -ne 1 ] || [ "$(tail -n 1 "$out")" != "fetch-pattern: refused: D: not an empty directory" ] ||
	[ "$(cat "$tmp/cwd$n/D")" != kept ]; then
	fail "a directory that is already there is refused: want exit 1 and D untouched, got exit $rc"
else
	echo "ok   a directory that is already there is refused, and left as it was"
fi

# U9: the plugin's manifests.
m=$root/.claude-plugin/marketplace.json
p=$root/.claude-plugin/plugin.json
if ! jq -e . "$m" >/dev/null 2>"$out" || ! jq -e . "$p" >/dev/null 2>>"$out"; then
	fail "the plugin's manifests do not parse"
else
	: >"$out"
	jq -e '(.name | type == "string") and (.owner.name | type == "string") and (.plugins | length == 1)
		and (.plugins[0].name == "placket") and (.plugins[0].source == "./")' "$m" >/dev/null ||
		echo "marketplace.json: not one plugin, placket, whose source is the repository root, ./, in a named marketplace with an owner" >>"$out"
	jq -e '(.name == "placket") and (.description | type == "string") and (has("version") | not)
		and (has("skills") | not) and (has("mcpServers") | not)' "$p" >/dev/null ||
		echo "plugin.json: not the plugin placket, described, with no version, no skill list and no MCP server" >>"$out"
	[ ! -e "$root/.mcp.json" ] || echo ".mcp.json: the plugin's root holds an MCP server" >>"$out"
	if [ -s "$out" ]; then
		fail "the plugin's manifests do not carry what Claude Code reads"
	else
		echo "ok   the marketplace lists one plugin, placket, from the repository root, whose plugin.json has no version, beside both skills"
	fi
fi

exit $failed
