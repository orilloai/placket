#!/bin/sh
# The GitHub merge applier: an agent's merge happens only once Placket released it, a person in the
# deciding role accepted it, and the pull request is still exactly what was released.
#
# Three invented parties share one Accord: an agent, which proposes merges; a decider, a person who
# answers each one; and the applier, apply.sh, which alone may merge under the repository's ruleset
# (custody.md). This script founds that Accord, and a second one whose terms pass a merge with no
# one asked, writes the labels config, plays the agent and the decider, runs apply.sh against the
# stand-in gh, and then follows the auditor's path of README.md from a merge commit back to its
# receipt.
#
# Run it as it is: `sh pattern.sh`. It uses the placket binary and jq on PATH, and the stand-in gh in
# stand-in/, which answers from its fixtures in place of GitHub and records each call. It makes
# everything else it needs in the current directory, keeps its labels config and its state beside
# this file (XDG_CONFIG_HOME and XDG_STATE_HOME), starts its own windows on sockets under $TMPDIR,
# and stops them before it exits. Here every party runs as one user; custody.md says which accounts
# a real run keeps apart. See README.md.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
export XDG_CONFIG_HOME="$here/config" XDG_STATE_HOME="$here/state"
PATH=$here/stand-in:$PATH # CHANGE: remove this line, so that gh is GitHub's own
export PATH
repo=example/widgets # CHANGE: the repository the applier serves, as owner/name, spelled as GitHub does

# A unix socket's path is short on some systems: the sockets live in a directory of their own.
sock_dir=$(mktemp -d "${TMPDIR:-/tmp}/plk.XXXXXX")
windows=
cleanup() {
	for pid in $windows; do
		kill "$pid" 2>/dev/null || :
		wait "$pid" 2>/dev/null || :
	done
	rm -rf "$sock_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

step() { echo; echo "== $*"; }
pause() { sleep 0.1 2>/dev/null || sleep 1; }
say() {
	who=$1
	shift
	echo "$who: $*"
}
ref() { read -r r <"$1.ref" && echo "$r"; }
yes_no() { if "$@" >/dev/null 2>&1; then echo yes; else echo no; fi; }

# found NAME ROLE...: founds the Accord NAME, one party for each ROLE with the terms NAME/ROLE.terms,
# starts its window on $sock_dir/NAME, and has each party view and accept its own view, the last
# acceptance founding it. Its id is in NAME/accord.
found() {
	n=$1
	shift
	roles=$(printf '"%s",' "$@")
	echo "{\"roles\":[${roles%,}],\"principals\":[{\"principal\":\"$(ref principal)\",\"roles\":[${roles%,}]}]}" >"$n/policy.json"
	mkdir -p "$n/openings"
	set_args=
	for a; do
		placket declare --key "$a.key" --terms "$n/$a.terms" >"$n/$a.declaration"
		placket seal --key "$a.key" --declaration "$n/$a.declaration" --out "$n/openings" >"$n/$a.seal" 2>/dev/null
		placket bind --key principal.key --actor "$(ref "$a")" --role "$a" --mediator "$(ref mediator)" \
			--until 2099-01-01T00:00:00Z --policy "$n/policy.json" >"$n/$a.binding"
		set_args="$set_args --seal $n/$a.seal --binding $n/$a.binding"
	done
	echo "{\"actor\":\"$(ref mediator)\",\"operator\":\"$(ref operator)\",\"runs\":\"process\"}" >"$n/mediator.json"
	placket card --default >"$n/card.json" 2>/dev/null
	# shellcheck disable=SC2086 # the seal and binding flags, split on purpose
	placket founding --json --policy "$n/policy.json" --mediator "$n/mediator.json" --card "$n/card.json" \
		$set_args >"$n/founding.out" 2>/dev/null
	jq '.founding' "$n/founding.out" >"$n/founding.json"
	jq -r '.accord' "$n/founding.out" >"$n/accord"
	placket window --key mediator.key --founding "$n/founding.json" --openings "$n/openings" \
		--socket "$sock_dir/$n" --journal "$n/journal" 2>"$n/window.log" &
	windows="$windows $!"
	i=0
	until grep -q '^operator ' "$n/window.log" 2>/dev/null; do
		i=$((i + 1))
		[ "$i" -lt 300 ] || { echo "the window of $n did not start"; cat "$n/window.log"; exit 1; }
		pause
	done
	for a; do
		placket view --key "$a.key" --socket "$sock_dir/$n" --accord "$(cat "$n/accord")" >/dev/null 2>&1
		placket accept --key "$a.key" --socket "$sock_dir/$n" --accord "$(cat "$n/accord")" >/dev/null 2>&1
	done
}

step "1. keys, terms and the two Accords"
for k in mediator operator principal agent applier decider; do
	placket key new --out "$k.key" >"$k.ref"
done
mkdir -p asked passed
# The agent's merges are asked of the decider, and no judge may pass one, since a merge cannot be
# undone. Its notes are asked too, so the applier is handed a kind it does not apply.
cat >asked/agent.terms <<'T'
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["merge"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["note"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","comment"]}}]}
T
cat >asked/applier.terms <<'T'
{"terms":[{"stance":"want","dir":"get","with":["role:agent"],"select":{"kind":["merge","note"]}}]}
T
cat >asked/decider.terms <<'T'
{"terms":[{"stance":"want","dir":"give","with":["role:agent"],"select":{"kind":["answer","comment"]}}]}
T
# The second Accord's terms pass a merge with no one asked, so its release carries no answer call.
cat >passed/agent.terms <<'T'
{"terms":[{"stance":"want","dir":"give","with":["role:applier"],"select":{"kind":["merge"]}}]}
T
cat >passed/applier.terms <<'T'
{"terms":[{"stance":"want","dir":"get","with":["role:agent"],"select":{"kind":["merge"]}}]}
T
found asked agent applier decider
found passed agent applier
say asked "founded: the agent's merges are asked of role decider"
say passed "founded: the agent's merges pass by the terms"

step "2. the labels config"
mkdir -p "$XDG_CONFIG_HOME/placket"
# CHANGE: your own mediators: the applier's key on each Accord whose releases it applies, labelled.
cat >"$XDG_CONFIG_HOME/placket/mediators.json" <<C
{"mediators":[
 {"label":"merge","accord":"$(cat asked/accord)","key":"$PWD/applier.key","founding":"$PWD/asked/founding.json","socket":"$sock_dir/asked"},
 {"label":"passed","accord":"$(cat passed/accord)","key":"$PWD/applier.key","founding":"$PWD/passed/founding.json","socket":"$sock_dir/passed"}]}
C
placket mediators --json | jq -r '.mediators[] | "mediators: \(.label)"'
state=$(placket mediators --json | jq -r '.mediators[] | select(.label == "merge") | .state')

asked=$(cat asked/accord)
as() { echo "--key $1.key --socket $sock_dir/asked --accord $asked"; }
# body FILE PR [BASE [HEAD [REPO]]]: writes the body of a merge of pull request PR into FILE, by
# default into the base and at the head GitHub's fixture gives it.
body() {
	f=$here/stand-in/fixtures/repos/$repo/pulls/$2.json
	[ -f "$f" ] || f=/dev/null
	jq -nc --arg repo "${5:-$repo}" --argjson pr "$2" --arg base "${3:-}" --arg head "${4:-}" '(input? // {}) as $p
		| {$repo, $pr, base: (if $base == "" then $p.base.ref else $base end), head: (if $head == "" then $p.head.sha else $head end)}' "$f" >"$1"
}
# propose WHAT FILE [GIVE FLAGS]: the agent gives FILE to the applier, data of kind merge unless the
# flags say otherwise, and the terms ask it of the decider; the question joins asked.list.
: >asked.list
propose() {
	what=$1 f=$2
	shift 2
	rc=0
	# shellcheck disable=SC2046 # as prints flags
	placket give $(as agent) --json --keep --to "$(ref applier)" --kind merge --as data --file "$f" "$@" >"$f.give" 2>/dev/null || rc=$?
	[ "$rc" -eq 4 ] || { echo "the give of $f was not asked: exit $rc"; exit 1; }
	jq -r '.question' "$f.give" >>asked.list
	say agent "proposes $what"
}
# decide: the decider accepts every question of asked.list, which releases each to the applier.
decide() {
	k=0
	while read -r q; do
		# shellcheck disable=SC2046 # as prints flags
		placket answer $(as decider) --json --question "$q" >answer.out 2>/dev/null
		k=$((k + 1))
	done <asked.list
	: >asked.list
	say decider "accepts $k, each released to the applier"
}
calls=$here/stand-in/state/calls.jsonl
events=$here/stand-in/state/events
mkdir -p "$here/stand-in/state"
: >"$calls"
: >"$events"
# applier ARGS...: runs apply.sh, each of its lines, standard error too, after "applier: ", with
# each call it makes of the stand-in, as the stand-in says it, in the order made.
applier() {
	GH_STAND_IN_SAY=yes sh "$here/apply.sh" "$@" 3>&1 2>&1 | sed '/^gh: /!s/^/applier: /'
}
# kept FILE: the applier's kept delivery of the merge whose body FILE holds, as apply.sh - takes it.
kept() { jq -c --slurpfile b "$1" 'select(.kind == "merge" and .data == $b[0]) | {delivery: .}' "$state/kept.jsonl"; }
pull() { jq -r "$1" "$here/stand-in/state/pulls/$repo/$2.json"; }

step "3. the agent proposes, the decider accepts, and the applier applies or refuses each release"
body pr7.json 7
propose "pull request 7, at its head" pr7.json
body pr8.json 8
propose "pull request 8, whose head a push moves after the applier reads it" pr8.json
echo "GET repos/$repo/pulls/8 push 1e4e888ac66f8dd41e00c5a7ac36a32a9950d271" >>"$events"
body repo.json 7 main "" example/other
propose "pull request 7 of another repository" repo.json
body pr.json 99 main 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
propose "a pull request GitHub does not have" pr.json
body base.json 9 release
propose "pull request 9, into another base" base.json
body head.json 9 main 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
propose "pull request 9, at another head" head.json
body pr9.json 9
jq -c '.why = "tidy"' pr9.json >shape.json
propose "pull request 9, with a member too many" shape.json
echo '"merge pull request 9"' >string.json
propose "data that is a string" string.json
echo "merge pull request 9" >text.txt
propose "a text" text.txt --as text
printf 'merge pull request 9' >raw.bin
propose "raw bytes" raw.bin --as raw
propose "a note whose body reads like a merge" pr9.json --kind note
body pr10.json 10
propose "pull request 10, merged already" pr10.json
body pr11.json 11
propose "pull request 11, closed" pr11.json
body pr12.json 12
propose "pull request 12, into a base whose ruleset leaves the applier out" pr12.json
body pr13.json 13
propose "pull request 13, whose merge call fails and merges nothing" pr13.json
echo "PUT repos/$repo/pulls/13/merge fail 502" >>"$events"
body pr14.json 14
propose "pull request 14, whose merge call merges and its answer is lost" pr14.json
echo "PUT repos/$repo/pulls/14/merge lose 502" >>"$events"
decide
applier merge "$repo"

step "4. what the applied release left on GitHub"
c7=$(pull .merge_commit_sha 7)
jq -c --arg r "repos/$repo" --arg c "$c7" 'select(.path == "\($r)/issues/7/comments" or .path == "\($r)/statuses/\($c)")' "$calls" >pr7.calls
comment=$(jq -c 'select(.method == "POST" and (.path | endswith("/comments")))' pr7.calls)
status=$(jq -c 'select(.method == "POST" and (.path | contains("/statuses/")))' pr7.calls)
kept pr7.json >pr7.kept
say gh "pull request 7 merged at the body's head: $(yes_no jq -e --arg r "$repo" --slurpfile k pr7.kept 'select(.method == "PUT" and .path == "repos/\($r)/pulls/7/merge") | .input.sha == $k[0].delivery.data.head' "$calls")"
say gh "the comment came before the status: $(yes_no jq -s -e '.[0].path | endswith("/comments")' pr7.calls)"
say gh "the comment holds the delivery, as one line of a code block: $(yes_no jq -e --slurpfile k pr7.kept '.input.body | startswith("    {") and (ltrimstr("    ") | fromjson) == $k[0].delivery' <<E
$comment
E
)"
say gh "the status is on the merge commit: $(yes_no jq -e --arg c "$c7" '.path | endswith("/statuses/" + $c)' <<E
$status
E
)"
echo "$status" | jq -r '"gh: its context is \(.input.context), its state \(.input.state)"'
say gh "its description is the receipt id: $(yes_no jq -e --slurpfile k pr7.kept '.input.description == $k[0].delivery.receiptId' <<E
$status
E
)"
say gh "its target is the comment: $(yes_no jq -e --arg u "$(jq -r '.html_url' "$here/stand-in/state/comments/1001.json")" '.input.target_url == $u' <<E
$status
E
)"
say gh "it holds neither the body nor the salt: $(yes_no jq -e --slurpfile k pr7.kept '$k[0].delivery as $d
	| [.input[] | strings] | all(contains($d.salt) or contains($d.data.head) or contains($d.data | tojson) | not)' <<E
$status
E
)"

step "5. the same release again, and altered"
say pattern "pull request 7's kept delivery, piped again"
applier merge "$repo" - <pr7.kept
# placket get refuses a delivery whose answer call is not the one its receipt names, so only an
# altered delivery reaches the applier this way; it checks the call all the same.
say pattern "the delivery, without its answer"
jq -c '.delivery |= del(.answer)' pr7.kept | applier merge "$repo" -
say pattern "the delivery, its answer a change"
jq -c '.delivery.answer.action = "changed"' pr7.kept | applier merge "$repo" -
say pattern "the delivery, its answer's from the agent's key"
jq -c --arg k "$(ref agent)" '.delivery.answer.from = $k' pr7.kept | applier merge "$repo" -
# The base is any name git takes for a branch's: one with an accent and a plus passes the body's
# check, and is refused only as applied; one git refuses is refused as a body.
say pattern "the delivery, its base a branch's name with an accent and a plus"
jq -c '.delivery.data.base = "feature/café+1"' pr7.kept | applier merge "$repo" -
say pattern "the delivery, its base a name no branch can have"
jq -c '.delivery.data.base = "main..next"' pr7.kept | applier merge "$repo" -
say pattern "two deliveries on one input"
cat pr7.kept pr7.kept | applier merge "$repo" -

step "6. a release the terms passed, with no one asked"
rc=0
placket give --key agent.key --socket "$sock_dir/passed" --accord "$(cat passed/accord)" --json \
	--to "$(ref applier)" --kind merge --as data --file pr7.json >give.out 2>/dev/null || rc=$?
say agent "gives a merge of pull request 7: $(jq -r '.outcome.word' give.out), exit $rc"
applier passed "$repo"

step "7. GitHub fails partway, and the applier stops for a person"
for n in 15 16 17 23 18 19 20 22; do body "pr$n.json" "$n"; done
propose "pull request 15, while the applier's installation no longer reaches the repository" pr15.json
propose "pull request 16, while GitHub limits the applier's rate" pr16.json
propose "pull request 17, whose comment GitHub does not take" pr17.json
propose "pull request 23, whose status GitHub does not take" pr23.json
propose "pull request 18, retargeted to next after the applier reads it" pr18.json
propose "pull request 19, which merges and cannot be read again" pr19.json
propose "pull request 20, whose merge call is answered as made and merges nothing" pr20.json
propose "pull request 22, while a person stops the applier" pr22.json
decide
cat >>"$events" <<E
GET repos/$repo/pulls/15 fail 404
GET repos/$repo fail 404
PUT repos/$repo/pulls/16/merge fail 429
POST repos/$repo/issues/17/comments fail 500
POST repos/$repo/statuses/$(jq -r '.merge_commit_sha' "$here/stand-in/fixtures/repos/$repo/pulls/23.json") fail 500
GET repos/$repo/pulls/18 retarget next
GET repos/$repo/pulls/19 pass
GET repos/$repo/pulls/19 fail none
PUT repos/$repo/pulls/20/merge hollow
GET repos/$repo/pulls/22 hold
E
# Each stop leaves the releases queued after it in the queue, for the next run.
for run in 15 16 17 23 18 19 20; do
	applier merge "$repo"
done
# A person stops the applier with TERM while GitHub reads pull request 22: the stand-in holds the read
# until the signal is sent, and the applier stops once the read ends. It runs in the background, so
# its own process id is the one signalled; an INT would be ignored by a background job of this shell.
GH_STAND_IN_SAY=yes sh "$here/apply.sh" merge "$repo" >interrupted.out 2>&1 3>&1 &
pid=$!
i=0
until [ -f "$here/stand-in/state/held" ]; do
	i=$((i + 1))
	[ "$i" -lt 300 ] || { echo "the stand-in did not hold the read of pull request 22"; exit 1; }
	pause
done
say pattern "TERM sent to the applier while GitHub reads pull request 22"
kill -TERM "$pid"
: >"$here/stand-in/state/go"
rc=0
wait "$pid" || rc=$?
sed '/^gh: /!s/^/applier: /' interrupted.out
say pattern "the applier exited $rc"
# unkeep N: takes pull request N's receipt id out of applied.jsonl, as README.md says to do by hand.
unkeep() {
	id=$(kept "pr$1.json" | jq -r '.delivery.receiptId')
	grep -Fvx "\"$id\"" "$here/applied.jsonl" >applied.rest || :
	cat applied.rest >"$here/applied.jsonl"
}
say pattern "pull request 15, piped again from the kept delivery once the repository reads"
kept pr15.json | applier merge "$repo" -
say pattern "pull request 16, its receipt id taken out of applied.jsonl, and piped again"
unkeep 16
kept pr16.json | applier merge "$repo" -
say pattern "pull request 17's comment and status, posted by hand from the kept delivery"
kept pr17.json | jq -c '{body: ("    " + (.delivery | tojson))}' | gh api -X POST "repos/$repo/issues/17/comments" --input - >hand.json
kept pr17.json | jq -c --slurpfile c hand.json '{state: "success", context: "placket", description: .delivery.receiptId, target_url: $c[0].html_url}' |
	gh api -X POST "repos/$repo/statuses/$(pull .merge_commit_sha 17)" --input - >/dev/null
say gh "statuses on pull request 17's merge commit: $(gh api "repos/$repo/commits/$(pull .merge_commit_sha 17)/statuses" | jq length)"
say gh "pull request 18 merged into $(pull .base.ref 18), with $(gh api "repos/$repo/commits/$(pull .merge_commit_sha 18)/statuses" | jq length) statuses"
say pattern "pull request 22, piped again from the kept delivery, since nothing of it was done"
kept pr22.json | applier merge "$repo" -

step "8. a repository spelled otherwise than GitHub spells it"
body case.json 21 main "" Example/Widgets
propose "pull request 21 of Example/Widgets" case.json
decide
applier merge Example/Widgets

step "9. the auditor, from a merge commit back to its receipt"
# Anyone who may write statuses can set one named placket. Here, newer than the applier's own on pull
# request 7's merge commit: thirty that link nothing; one that links a comment of pull request 14's
# delivery; and one that names the receipt of the agent's proposal of pull request 7, the ask, and
# links a comment of that part, with its salt, under that receipt's id.
# The thirty go straight into the stand-in's record of statuses, as thirty such calls would.
jq -nc 'range(30) | {state: "success", context: "placket", description: "merged", target_url: "https://example.com/"}' \
	>>"$here/stand-in/state/statuses/$c7.jsonl"
kept pr14.json | jq -c '{body: ("    " + (.delivery | tojson))}' | gh api -X POST "repos/$repo/issues/7/comments" --input - >other.comment
kept pr14.json | jq -c --slurpfile c other.comment '{state: "success", context: "placket", description: .delivery.receiptId, target_url: $c[0].html_url}' |
	gh api -X POST "repos/$repo/statuses/$c7" --input - >/dev/null
jq -c --slurpfile g pr7.json.give --slurpfile b pr7.json '.message | @base64d | fromjson | select(.parts[0].data == $b[0])' \
	"agent.key.state/$(cut -c8- asked/accord)/kept.jsonl" >asked.message
jq -c --slurpfile g pr7.json.give '{receiptId: $g[0].question, kind: "merge", messageId, data: .parts[0].data,
	salt: .parts[0].metadata["https://placket.tech/crossing/0.1"].salt, receipt: $g[0].outcome.receipt} | {body: ("    " + tojson)}' \
	asked.message | gh api -X POST "repos/$repo/issues/7/comments" --input - >asked.comment
jq -c --slurpfile g pr7.json.give '{state: "success", context: "placket", description: $g[0].question, target_url: .html_url}' asked.comment |
	gh api -X POST "repos/$repo/statuses/$c7" --input - >/dev/null
say pattern "32 more statuses named placket on pull request 7's merge commit"

# CHANGE: the Accord, the founding set, its acceptances and the journal the operator handed you, the
# deciding role, and the merge commit you audit.
accord=$(cat asked/accord) founding=asked/founding.json acceptances=asked/acceptances.json journal=asked/journal
role=decider
commit=$c7
# Step 1: the pull request the commit merged, as GitHub lists it.
gh api "repos/$repo/commits/$commit/pulls" |
	jq -c --arg c "$commit" '[.[] | select(.merged_at != null and .merge_commit_sha == $c)] | first' >merged.json
# Step 2: every status named placket on the commit, newest first, every page of them.
gh api --paginate "repos/$repo/commits/$commit/statuses" | jq -c -s 'add | map(select(.context == "placket"))' >statuses.json
say auditor "statuses named placket on the commit: $(jq length statuses.json)"
jq -c --arg role "$role" '[.bindings[] | select(.roles | index($role)) | .actor]' "$founding" >deciders.json
# audit I: steps 3 to 6 from the commit's I-th status named placket; it prints why the status is not
# this merge's record, or nothing when it is.
audit() {
	jq -c --argjson i "$1" '.[$i]' statuses.json >status.json
	n=$(jq -r '.number' merged.json)
	# Step 3: only a comment's number, from a link to a comment on the merged pull request.
	id=$(jq -r --arg p "https://github.com/$repo/pull/$n#issuecomment-" '.target_url | strings
		| select(startswith($p)) | ltrimstr($p) | select(test("^[0-9]+$"))' status.json)
	[ -n "$id" ] || { echo "it links no comment on pull request $n"; return; }
	gh api "repos/$repo/issues/comments/$id" | jq -r '.body | ltrimstr("    ")' >comment.json
	jq -e --slurpfile s status.json --slurpfile p merged.json '.receiptId == $s[0].description and (.data as $b | $p[0]
		| .base.repo.full_name == $b.repo and .number == $b.pr and .base.ref == $b.base and .head.sha == $b.head)' \
		comment.json >/dev/null 2>&1 || { echo "its comment's delivery is not of its receipt, or names another merge"; return; }
	# Step 4: verify, on the comment's delivery and its answer call.
	jq -c '.answer' comment.json >answer.json
	rc=0
	placket verify --json --accord "$accord" --founding "$founding" --acceptances "$acceptances" --receipts "$journal" \
		--delivery comment.json --answers answer.json >verify.json 2>verify.err || rc=$?
	[ "$rc" -eq 0 ] || { echo "verify exits $rc"; return; }
	# Step 5: the receipt is a release the deciding role's key accepted: verify witnessed its answer,
	# signed by that key, and the call verify matched to it is an accept.
	jq -e --slurpfile c comment.json --slurpfile k deciders.json --arg role "$role" '$c[0] as $d
		| [.decisions[] | select(.receiptId == $d.receiptId) | .line
			| capture("^(?<id>[^ ]+) witnessed: signed by key (?<key>[^ ]+), which principal [^ ]+ bound to role (?<roles>[^ ]+)$")]
		| length == 1 and (.[0].key as $key | $k[0] | index($key)) != null and (.[0].roles | split(",") | index($role)) != null
		and $d.answer.action == "accepted" and $d.answer.from == .[0].key' verify.json >/dev/null 2>&1 ||
		{ echo "its receipt is not a release that role $role accepted"; return; }
	jq -e --slurpfile c comment.json '.body == "body: the part of receipt \($c[0].receiptId) is the one it names"' verify.json >/dev/null ||
		{ echo "verify does not say its delivery is its receipt's part"; return; }
}
i=0
: >audit.log
while :; do
	[ "$i" -lt "$(jq length statuses.json)" ] || { say auditor "no status named placket is this merge's record"; exit 1; }
	why=$(audit "$i")
	[ -n "$why" ] || break
	echo "$why" >>audit.log
	i=$((i + 1))
done
uniq -c audit.log | sed 's/^ *\([0-9]*\) /auditor: \1 refused: /'
say auditor "status $((i + 1)), newest first, is this merge's record"
jq -r '.valid | sub(" by mediator .*"; "") | "verify: \(.)"' verify.json
jq -r '"verify: \(.terms)"' verify.json
jq -r --slurpfile c comment.json --arg d "$(ref decider)" --arg p "$(ref principal)" '.decisions[] | select(.receiptId == $c[0].receiptId) | .line
	| sub("^[^ ]+"; "verify: <the receipt>") | sub($d; "<the decider'\''s key>") | sub($p; "<the principal>")' verify.json
jq -r --slurpfile c comment.json '.body | sub($c[0].receiptId; "<the receipt>") | "verify: \(.)"' verify.json
say auditor "the body names this merge's repository, pull request, base and head: $(yes_no jq -e --slurpfile p merged.json '.data as $b
	| $p[0] | .base.repo.full_name == $b.repo and .number == $b.pr and .base.ref == $b.base and .head.sha == $b.head' comment.json)"
