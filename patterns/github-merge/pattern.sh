#!/bin/sh
# The GitHub merge applier: an agent's merge happens only when Placket released it, a person with
# the deciding role said yes, and the pull request is still exactly what was released.
#
# Three invented parties share one Accord: an agent, which proposes merges; a decider, a person who
# answers each one; and the applier, apply.sh, which alone may merge under the repository's ruleset
# (custody.md). This script founds that Accord, and a second one whose terms pass a merge with no
# one asked, sets up the labels config, plays the agent and the decider, runs apply.sh, and then
# follows the auditor's path of README.md from the merge commit back to its receipt.
#
# Run it as it is: `sh pattern.sh`. It uses the placket binary and jq on PATH, and the stand-in gh in
# stand-in/, which answers from its fixtures and records each call, in place of GitHub. It makes
# everything else it needs in the current directory, keeps its labels config and its state beside
# this file (XDG_CONFIG_HOME and XDG_STATE_HOME), starts its own windows on sockets under $TMPDIR,
# and stops them before it exits. Here every party runs as one user; custody.md says which accounts a
# real run keeps apart. See README.md.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
export XDG_CONFIG_HOME="$here/config" XDG_STATE_HOME="$here/state"
PATH=$here/stand-in:$PATH # CHANGE: remove this line, so that gh is the real one
export PATH
repo=example/widgets # CHANGE: the repository the applier serves, as owner/name

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
ref() { cat "$1.ref"; }
# yes_no CMD...: yes when CMD succeeds, else no; CMD prints nothing.
yes_no() { if "$@" >/dev/null 2>&1; then echo yes; else echo no; fi; }

# found NAME ROLE...: founds the Accord NAME, of one party for each ROLE, each with the terms
# NAME/ROLE.terms and the key ROLE.key, starts its window on the socket NAME.sock under $sock_dir,
# and has every party view and accept its view. Its id is in NAME/accord.
found() {
	n=$1
	shift
	roles=$(printf '"%s",' "$@")
	roles="[${roles%,}]"
	echo "{\"roles\":$roles,\"principals\":[{\"principal\":\"$(ref principal)\",\"roles\":$roles}]}" >"$n/policy.json"
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
		--socket "$sock_dir/$n.sock" --journal "$n/journal" 2>"$n/window.log" &
	windows="$windows $!"
	i=0
	until grep -q '^operator ' "$n/window.log" 2>/dev/null; do
		i=$((i + 1))
		[ "$i" -lt 300 ] || { echo "the window of $n did not start"; cat "$n/window.log"; exit 1; }
		pause
	done
	# The Accord is proposed: each party views its own view and accepts it, and the last acceptance
	# founds it.
	for a; do
		placket view --key "$a.key" --socket "$sock_dir/$n.sock" --accord "$(cat "$n/accord")" >/dev/null 2>&1
		placket accept --key "$a.key" --socket "$sock_dir/$n.sock" --accord "$(cat "$n/accord")" >/dev/null 2>&1
	done
}
as() { echo "--key $1.key --socket $sock_dir/asked.sock --accord $(cat asked/accord)"; }

# propose WHAT FILE [GIVE FLAGS]: the agent proposes FILE, data of kind merge unless the flags say
# otherwise, to the applier, and says WHAT it proposed; the question's id joins asked.list.
propose() {
	what=$1 f=$2
	shift 2
	rc=0
	# shellcheck disable=SC2046 # as prints flags
	placket give $(as agent) --json --to "$(ref applier)" --kind merge --as data --file "$f" "$@" >give.out 2>/dev/null || rc=$?
	[ "$rc" -eq 4 ] && [ "$(jq -r '.outcome.word' give.out)" = ask ] || { echo "the give of $f was not asked: exit $rc"; exit 1; }
	jq -r '.outcome.receiptId' give.out >>asked.list
	say agent "proposes $what: ask"
}
# decide: the decider accepts each question of asked.list, in order, which releases it to the
# applier, and empties the list.
decide() {
	k=0
	while read -r q; do
		# shellcheck disable=SC2046 # as prints flags
		placket answer $(as decider) --json --question "$q" >answer.out 2>/dev/null
		[ "$(jq -r '.release.word' answer.out)" = released ] || { echo "the answer to $q released nothing"; exit 1; }
		k=$((k + 1))
	done <asked.list
	: >asked.list
	say decider "accepts $k, each released to the applier"
}
calls() { wc -l <"$here/stand-in/state/calls.jsonl" | tr -d ' '; }
# show_calls FROM: the stand-in's calls after the first FROM, each its method and path, a commit's
# hash in the path shown as <commit>.
show_calls() {
	tail -n "+$(($1 + 1))" "$here/stand-in/state/calls.jsonl" |
		jq -r '"gh: \(.method) \(.path | sub("/[0-9a-f]{40}$"; "/<commit>"))"'
}
applier() { sh "$here/apply.sh" "$@" 2>&1 | sed 's/^/applier: /'; }
# merge_of N: the merge commit the stand-in's fixture gives pull request N.
merge_of() { jq -r '.merge_commit_sha' "$here/stand-in/fixtures/repos/$repo/pulls/$1.json"; }
# kept_pr N: the applier's kept delivery of a merge of pull request N, as apply.sh - takes it.
kept_pr() {
	jq -c --argjson n "$1" 'select(.kind == "merge" and (.data | type == "object" and .pr == $n)) | {delivery: .}' "$state/kept.jsonl"
}

step "1. keys, terms and the founding sets"
for k in mediator operator principal agent applier decider; do
	placket key new --out "$k.key" >"$k.ref"
done
mkdir -p asked unasked
# The agent's merges are asked of the decider, and no judge may pass one: a merge cannot be undone.
# Its notes are asked too, so the applier is handed a kind it does not apply.
cat >asked/agent.terms <<'T'
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["merge"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["note"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","comment"]}}]}
T
cat >asked/applier.terms <<'T'
{"terms":[
 {"stance":"want","dir":"get","with":["role:agent"],"select":{"kind":["merge","note"]}}]}
T
cat >asked/decider.terms <<'T'
{"terms":[
 {"stance":"want","dir":"give","with":["role:agent"],"select":{"kind":["answer","comment"]}}]}
T
# The second Accord's terms pass a merge with no one asked, so its release carries no answer call.
cat >unasked/agent.terms <<'T'
{"terms":[
 {"stance":"want","dir":"give","with":["role:applier"],"select":{"kind":["merge"]}}]}
T
cat >unasked/applier.terms <<'T'
{"terms":[
 {"stance":"want","dir":"get","with":["role:agent"],"select":{"kind":["merge"]}}]}
T
found asked agent applier decider
found unasked agent applier
say asked "founded, its merges asked of role decider"
say unasked "founded, its merges passed by the terms"

step "2. the labels config"
mkdir -p "$XDG_CONFIG_HOME/placket"
# CHANGE: your own mediators: the applier's key on each Accord whose releases it applies, labelled.
cat >"$XDG_CONFIG_HOME/placket/mediators.json" <<C
{"mediators":[
 {"label":"merge","accord":"$(cat asked/accord)","key":"$PWD/applier.key","founding":"$PWD/asked/founding.json","socket":"$sock_dir/asked.sock"},
 {"label":"unasked","accord":"$(cat unasked/accord)","key":"$PWD/applier.key","founding":"$PWD/unasked/founding.json","socket":"$sock_dir/unasked.sock"}]}
C
placket mediators --json | jq -r '.mediators[] | "mediators: \(.label)"'

# The pull request the agent asks to merge, as GitHub holds it, and the commit a later push moves its
# head to. Both are invented; the stand-in's fixture holds the first.
first=$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/7.json")
second=2b7d4f6a8c0e1a3c5e7b9d1f3a5c7e9b0d2f4a6c
body() { jq -nc --arg repo "$1" --argjson pr "$2" --arg base "$3" --arg head "$4" '{$repo, $pr, $base, $head}'; }

step "3. one refused for each check, the first because the head moved"
body "$repo" 7 main "$first" >moved.json
propose "pull request 7 at its head" moved.json
# Once the head has moved, each of the next five differs from the pull request in one member: the
# old head, another base, a base whose name has an accent and a plus, as a branch's name may, another
# number and another repository.
body "$repo" 7 main "$first" >head.json
propose "pull request 7 at its old head" head.json
body "$repo" 7 release "$second" >base.json
propose "pull request 7 into another base" base.json
body "$repo" 7 "feature/café+1" "$second" >plus.json
propose "pull request 7 into a base with an accent and a plus" plus.json
body "$repo" 99 main "$second" >pr.json
propose "a pull request GitHub does not have" pr.json
body example/other 7 main "$second" >repo.json
propose "pull request 7 of another repository" repo.json
# Wrong bodies: a member too many, a base no branch can have, data that is no object, a text, raw
# bytes, and a note that reads like a merge.
body "$repo" 7 main "$second" | jq -c '.why = "tidy"' >shape.json
propose "a body with a member too many" shape.json
body "$repo" 7 "main..next" "$second" >dots.json
propose "a base no branch can have" dots.json
echo '"merge pull request 7"' >string.json
propose "data that is a string" string.json
echo "merge pull request 7" >text.txt
propose "a text" text.txt --as text
printf 'merge pull request 7' >raw.bin
propose "raw bytes" raw.bin --as raw
body "$repo" 7 main "$second" >note.json
propose "a note that reads like a merge" note.json --kind note
decide
mkdir -p "$here/stand-in/state"
touch "$here/stand-in/state/calls.jsonl"
# The stand-in moves the head once the applier has read the pull request for the first release, as a
# push landing between its read and its merge would.
echo "$second" >"$here/stand-in/state/push-7"
from=$(calls)
applier merge "$repo"
show_calls "$from"
say gh "pull request 7 merged: $(yes_no jq -e '.merged' "$here/stand-in/state/pulls/$repo/7.json")"

step "4. a release of the new head, applied"
body "$repo" 7 main "$second" >good.json
propose "pull request 7 at its new head" good.json
decide
from=$(calls)
applier merge "$repo"
show_calls "$from"
tail -n "+$((from + 1))" "$here/stand-in/state/calls.jsonl" >applied.calls
# The delivery the applier fetched, as its state directory kept it.
state=$(placket mediators --json | jq -r '.mediators[] | select(.label == "merge") | .state')
jq -c 'select(.kind == "merge")' "$state/kept.jsonl" | tail -n 1 >kept.json
put() { jq -c "$1" applied.calls; }
put 'select(.method == "POST" and (.path | endswith("/comments")))' >comment.call
put 'select(.method == "POST" and (.path | contains("/statuses/")))' >status.call
merged=$(jq -r '.merge_commit_sha' "$here/stand-in/state/pulls/$repo/7.json")
url=$(jq -r '.html_url' "$here/stand-in/state/comments/1001.json")
say gh "the merge's sha is the body's head: $(yes_no jq -e --arg h "$second" 'select(.method == "PUT") | .input.sha == $h' applied.calls)"
say gh "the comment came before the status: $(yes_no jq -s -e '
	([to_entries[] | select(.value.path | endswith("/comments")) | .key] | first) as $c
	| ([to_entries[] | select(.value.path | contains("/statuses/")) | .key] | first) as $s
	| $c != null and $s != null and $c < $s' applied.calls)"
say gh "the comment holds the delivery: $(yes_no jq -e --slurpfile k kept.json '(.input.body | fromjson) == $k[0]' comment.call)"
say gh "as a code block, so nothing in it renders: $(yes_no jq -e '.input.body | startswith("    {")' comment.call)"
say gh "the status is on the merge commit: $(yes_no jq -e --arg m "$merged" '.path | endswith("/statuses/" + $m)' status.call)"
say gh "its context is $(jq -r '.input.context' status.call), its state $(jq -r '.input.state' status.call)"
say gh "its description is the receipt id: $(yes_no jq -e --slurpfile k kept.json '.input.description == $k[0].receiptId' status.call)"
say gh "its target is the comment: $(yes_no jq -e --arg u "$url" '.input.target_url == $u' status.call)"
say gh "it holds neither the body nor the salt: $(yes_no jq -e --slurpfile k kept.json '
	[.input[] | strings] | all(contains($k[0].salt) or contains($k[0].data.head) or contains($k[0].data | tojson) | not)' status.call)"

step "5. the same release again"
from=$(calls)
jq -c '{delivery: .}' kept.json | applier merge "$repo" -
say gh "calls: $(($(calls) - from))"

step "6. the same release, altered"
# placket get refuses a delivery whose answer call is not the one its receipt names, so only an
# altered delivery reaches the applier this way; it checks the call all the same. The message id is
# the giver's own, and the applier posts the delivery under its own name, so it takes only a plain id.
from=$(calls)
say pattern "the delivery, its message id a link and a mention"
jq -c '{delivery: (.messageId = "[see](https://example.com/x) @someone")}' kept.json | applier merge "$repo" -
say pattern "the delivery, its answer's action changed"
jq -c '{delivery: (.answer.action = "changed")}' kept.json | applier merge "$repo" -
say pattern "the delivery, its answer's from set to the agent's key"
jq -c --arg agent "$(ref agent)" '{delivery: (.answer.from = $agent)}' kept.json | applier merge "$repo" -
say pattern "the delivery, without its answer"
jq -c '{delivery: del(.answer)}' kept.json | applier merge "$repo" -
say gh "calls: $(($(calls) - from))"

step "7. a release the terms passed, with no one asked"
rc=0
placket give --key agent.key --socket "$sock_dir/unasked.sock" --accord "$(cat unasked/accord)" --json \
	--to "$(ref applier)" --kind merge --as data --file good.json >give.out 2>/dev/null || rc=$?
say agent "gives a merge: $(jq -r '.outcome.word' give.out), exit $rc"
from=$(calls)
applier unasked "$repo"
say gh "calls: $(($(calls) - from))"

step "8. GitHub fails partway"
body "$repo" 8 main "$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/8.json")" >down.json
propose "pull request 8, while GitHub cannot be read" down.json
body "$repo" 9 main "$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/9.json")" >lost.json
propose "pull request 9, whose merge's answer is lost" lost.json
decide
# The read of pull request 8 fails once, and the merge of pull request 9 merges and answers an error.
cat >>"$here/stand-in/state/faults" <<F
GET repos/$repo/pulls/8 503
PUT repos/$repo/pulls/9/merge 502+
F
from=$(calls)
applier merge "$repo"
show_calls "$from"
# The read failed before its id was kept, so the applier's own kept delivery, piped to it again,
# merges it (README.md, "When it stops partway").
from=$(calls)
kept_pr 8 | applier merge "$repo" -
show_calls "$from"

step "9. it stops, for a person"
# Each of these stops the applier partway, and the next run starts at the next one: a comment, or a
# status, GitHub does not take after the merge; a merge call answered as made while nothing merged; a
# pull request GitHub cannot read once merged; and the applier's credential refused at the read and
# at the merge call, as an App's token is once its hour is up.
for n in 13 14 15 16 17 18; do
	body "$repo" "$n" main "$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/$n.json")" >"stop-$n.json"
done
propose "pull request 13, whose comment GitHub does not take" stop-13.json
propose "pull request 14, whose status GitHub does not take" stop-14.json
propose "pull request 15, whose merge GitHub answers and does not make" stop-15.json
propose "pull request 16, which GitHub cannot read once merged" stop-16.json
propose "pull request 17, read with a lapsed credential" stop-17.json
propose "pull request 18, merged with a lapsed credential" stop-18.json
decide
cat >>"$here/stand-in/state/faults" <<F
POST repos/$repo/issues/13/comments 500
POST repos/$repo/statuses/$(merge_of 14) 500
PUT repos/$repo/pulls/15/merge hollow
GET repos/$repo/pulls/16 ok
GET repos/$repo/pulls/16 503
GET repos/$repo/pulls/17 401
PUT repos/$repo/pulls/18/merge 401
F
for n in 13 14 15 16 17 18; do
	from=$(calls)
	applier merge "$repo"
	show_calls "$from"
done
# The recoveries of README.md, "When it stops partway".
say pattern "pull request 13's comment and status, posted by hand from the kept delivery"
receipt=$(kept_pr 13 | jq -r '.delivery.receiptId')
from=$(calls)
jq -c --arg id "$receipt" 'select(.receiptId == $id) | {body: ("    " + tojson)}' "$state/kept.jsonl" |
	gh api -X POST "repos/$repo/issues/13/comments" --input - >hand.json
jq -nc --arg id "$receipt" --arg url "$(jq -r '.html_url' hand.json)" '{state: "success", context: "placket", description: $id, target_url: $url}' |
	gh api -X POST "repos/$repo/statuses/$(merge_of 13)" --input - >/dev/null
show_calls "$from"
say gh "statuses on pull request 13's merge commit: $(gh api "repos/$repo/commits/$(merge_of 13)/statuses" | jq length)"
say pattern "pull request 17, piped again once the credential is renewed"
from=$(calls)
kept_pr 17 | applier merge "$repo" -
show_calls "$from"
say pattern "pull request 18, its receipt id taken out of applied.jsonl and piped again"
receipt=$(kept_pr 18 | jq -r '.delivery.receiptId')
grep -Fvx "\"$receipt\"" "$here/applied.jsonl" >applied.rest
cat applied.rest >"$here/applied.jsonl"
from=$(calls)
kept_pr 18 | applier merge "$repo" -
show_calls "$from"

step "10. a pull request merged, or closed, before the applier's merge call"
# Pull request 7 merged in step 4, and pull request 12 was closed unmerged: neither is the applier's
# to merge or to record.
body "$repo" 7 main "$second" >again.json
propose "pull request 7, merged already" again.json
body "$repo" 12 main "$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/12.json")" >closed.json
propose "pull request 12, closed" closed.json
decide
from=$(calls)
applier merge "$repo"
show_calls "$from"

step "11. the base changed between the read and the merge"
# The stand-in's fixture lists main and next as the bases the applier's ruleset lets it merge into.
# Pull request 10 is retargeted to release, whose ruleset leaves the applier out, and pull request 11
# to next, another base the applier serves.
body "$repo" 10 main "$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/10.json")" >retarget.json
propose "pull request 10 into main" retarget.json
body "$repo" 11 main "$(jq -r '.head.sha' "$here/stand-in/fixtures/repos/$repo/pulls/11.json")" >served.json
propose "pull request 11 into main" served.json
decide
echo release >"$here/stand-in/state/retarget-10"
echo next >"$here/stand-in/state/retarget-11"
from=$(calls)
applier merge "$repo"
show_calls "$from"
for n in 10 11; do
	say gh "pull request $n merged: $(yes_no jq -e '.merged' "$here/stand-in/state/pulls/$repo/$n.json"), statuses on its merge commit: $(gh api "repos/$repo/commits/$(jq -r '.merge_commit_sha' "$here/stand-in/state/pulls/$repo/$n.json")/statuses" | jq length)"
done

step "12. the auditor, from the merge commit back to its receipt"
# CHANGE: the merge commit you audit, and the Accord, founding set, acceptances and journal its
# operator handed you.
commit=$(gh api "repos/$repo/pulls/7" | jq -r '.merge_commit_sha')
accord=$(cat asked/accord)
gh api "repos/$repo/commits/$commit/statuses" | jq -c '[.[] | select(.context == "placket")] | first' >status.json
receipt=$(jq -r '.description' status.json)
# Anyone who may write statuses can set one named placket, and its link: take only a comment's
# number, from a link to a pull request of this repository.
id=$(jq -r --arg p "https://github.com/$repo/pull/" '.target_url | strings
	| select(ascii_downcase | startswith($p | ascii_downcase))
	| capture("^https://github\\.com/[^/]+/[^/]+/pull/[0-9]+#issuecomment-(?<id>[0-9]+)$").id' status.json)
[ -n "$id" ] || { say auditor "the status links no comment on a pull request of $repo"; exit 1; }
gh api "repos/$repo/issues/comments/$id" | jq -r '.body' >comment.json
jq -c '.answer' comment.json >answer.json
say auditor "the status names the receipt of the comment's delivery: $(yes_no jq -e --arg r "$receipt" '.receiptId == $r' comment.json)"
rc=0
placket verify --accord "$accord" --founding asked/founding.json --acceptances asked/acceptances.json \
	--receipts asked/journal --delivery comment.json --answers answer.json >verify.out 2>&1 || rc=$?
say auditor "verify exits $rc"
grep '^valid: ' verify.out | sed 's/ by mediator .*//; s/^/verify: /'
grep '^terms: ' verify.out | sed 's/^/verify: /'
grep -q "^$receipt witnessed: signed by key $(ref decider), which principal $(ref principal) bound to role decider$" verify.out &&
	echo "verify: <the receipt> witnessed: signed by the decider's key, bound to role decider"
grep -q "^body: the part of receipt $receipt is the one it names$" verify.out &&
	echo "verify: body: the part of <the receipt> is the one it names"
gh api "repos/$repo/commits/$commit/pulls" | jq -c --arg c "$commit" '[.[] | select(.merged_at != null and .merge_commit_sha == $c)] | first' >pull.json
say auditor "the body names the pull request merged as this commit, its repo, number, base and head: $(yes_no jq -e --slurpfile p pull.json '
	.data as $b | $p[0] | .base.repo.full_name == $b.repo and .number == $b.pr and .base.ref == $b.base and .head.sha == $b.head' comment.json)"
