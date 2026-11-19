#!/bin/sh
# The GitHub merge applier. It merges a pull request only when Placket released a merge of it to the
# applier, a key the founding set binds to the deciding role accepted that release, and the pull
# request is, when it is read, open, unmerged and exactly what was released: the repository, the
# number, the base branch and the head commit. It merges at that head commit, so GitHub refuses the
# merge if the head moved since. Copy this file alone to run it for real; pattern.sh runs it on
# invented data. See README.md, and custody.md for the GitHub settings it rests on.
#
#   sh apply.sh LABEL OWNER/NAME [-]
#
# LABEL names the applier's mediator in the labels config, which placket finds where the caller's
# XDG_CONFIG_HOME, or its default, says; OWNER/NAME is the one repository it serves, as GitHub spells
# it. It sets nothing up. It fetches each release with `placket get --label LABEL --keep --json` and
# handles it, and stops once the queue is empty. With `-` it fetches nothing and handles the one
# delivery on standard input, as `placket get --json` printed it: placket checks none of that input,
# so give it only a line of the applier's own kept.jsonl (README.md, "When it stops partway").
#
# It prints one line for each release: `applied pr <n>`, or `refused pr <n> <reason>`, `-` standing
# for <n> when the body names no number. It never tries a refused release again. The reasons, in the
# order it checks them:
#   body         the delivery is not data of kind merge with exactly repo, pr, base and head, each
#                of its form;
#   unwitnessed  it carries no answer call, or the call is not an accept, or its from is not a key the
#                founding set's bindings give the deciding role;
#   applied      its receipt id is in applied.jsonl, beside this file;
#   repo         the body names another repository than OWNER/NAME, or GitHub's pull request is in
#                another, or spells it otherwise;
#   pr           GitHub has no such pull request in the repository;
#   merged       the pull request is merged already, by anyone;
#   closed       it is closed;
#   base, head   its base branch, or its head commit, is not the body's;
#   head-moved   GitHub refused the merge because the head moved after the read;
#   merge        GitHub refused the merge for another reason of its own, such as a ruleset, or did not
#                answer and the pull request is still unmerged.
# Where GitHub does not answer, refuses the applier's credential, or limits its rate, the applier
# stops instead, exit 1, with a line on standard error that says what to do (README.md, "When it
# stops partway"), so that one outage does not refuse every release queued after it.
set -eu

role=decider # CHANGE: the role whose accept releases a merge, as your Accord's terms name it
method=merge # CHANGE: merge, squash or rebase, as the repository allows
forever=no   # CHANGE: yes, to wait for releases until interrupted; no stops once the queue is empty

[ $# -eq 2 ] || { [ $# -eq 3 ] && [ "$3" = - ]; } || { echo "usage: sh apply.sh LABEL OWNER/NAME [-]" >&2; exit 2; }
label=$1 repo=$2 one=${3:-}
# An owner and a name, of the letters GitHub allows, so the repository is a plain path of its API.
echo "$repo" | grep -Eqx '[A-Za-z0-9-]+/[A-Za-z0-9._-]+' && [ "${repo#*/}" != . ] && [ "${repo#*/}" != .. ] ||
	{ echo "apply: $repo is not OWNER/NAME" >&2; exit 2; }

here=$(cd "$(dirname "$0")" && pwd)
applied=$here/applied.jsonl
tmp=$(mktemp -d "${TMPDIR:-/tmp}/apply.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
touch "$applied"

# The keys the founding set's bindings give the deciding role. The terms that name the role are
# sealed; the bindings are in the clear.
founding=$(placket mediators --json | jq -r --arg l "$label" '.mediators[] | select(.label == $l) | .founding')
[ -n "$founding" ] || { echo "apply: the labels config has no mediator labelled $label" >&2; exit 2; }
jq -c --arg role "$role" '[.bindings[] | select(.roles | index($role)) | .actor]' "$founding" >"$tmp/deciders.json"

refuse() { echo "refused pr $1 $2"; }
# stop TEXT: the applier stops, for a person to do what TEXT says.
stop() {
	echo "apply: $1" >&2
	exit 1
}
# status FILE: the HTTP code of the error gh printed into FILE, or none when GitHub did not answer.
status() { sed -n 's/.*(HTTP \([0-9][0-9]*\))$/\1/p' "$1" | tail -n 1 | grep . || echo none; }
# answer FILE: GitHub's answer, as the error gh printed into FILE gives it.
answer() { if [ "$(status "$1")" = none ]; then echo "no answer"; else echo "HTTP $(status "$1")"; fi; }
# read_pull FILE: GitHub's pull request, into FILE, its error into FILE.err.
read_pull() { gh api "repos/$repo/pulls/$pr" >"$1" 2>"$1.err"; }

# check: the first check of the delivery in $tmp/d.json it fails, of those placket's own record
# answers, then the pull request's number, or -, then its receipt id. placket get has refused a
# delivery whose answer call is not the one its receipt names; the call's signature rests on the
# mediator (README.md, "What it claims").
check() {
	jq -r --arg repo "$repo" --slurpfile keys "$tmp/deciders.json" --rawfile done "$applied" '
		def holds(f): (try f catch false) == true;
		def git_branch: type == "string" and length <= 255 and test("^[^\\x00-\\x20\\x7f~^:?*\\[\\\\]+$")
			and (test("\\.\\.|//|@\\{|(^|/)\\.|\\.lock(/|$)|[/.]$|^/|^@$") | not);
		.delivery as $d
		| ($d.data | if holds(type == "object" and (.pr | type == "number" and . >= 1 and . == floor and . < 1e9))
			then .pr | floor | tostring else "-" end) as $n
		| (if holds($d.kind == "merge" and ($d | has("text") or has("raw") | not) and ($d | tojson | length) <= 65000
				and ($d.receiptId | test("^sha256:[0-9a-f]{64}$"))
				and ($d.data | type == "object" and keys == ["base", "head", "pr", "repo"])
				and ($d.data.repo | type == "string" and test("^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$"))
				and $n != "-"
				and ($d.data.base | git_branch)
				and ($d.data.head | type == "string" and test("^([0-9a-f]{40}|[0-9a-f]{64})$"))) | not then "body"
			elif holds($d.answer.action == "accepted" and ($d.answer.from as $f | $keys[0] | any(.[]; . == $f))) | not then "unwitnessed"
			elif ("\"" + $d.receiptId + "\"") as $id | $done | split("\n") | any(.[]; . == $id) then "applied"
			elif $d.data.repo != $repo then "repo"
			else "ok" end) as $verdict
		| "\($verdict) \($n) \(if $verdict == "ok" then $d.receiptId else "-" end)"' "$tmp/d.json" 2>/dev/null ||
		echo "body - -"
}

# apply: handles the one delivery in $tmp/d.json.
apply() {
	read -r verdict pr id <<-EOF
		$(check)
	EOF
	[ "$verdict" = ok ] || { refuse "$pr" "$verdict"; return; }

	if ! read_pull "$tmp/pull.json"; then
		# A 404 is GitHub's answer for a pull request it does not have, and also for a repository the
		# applier's credential no longer reaches: only once the repository reads is it the first.
		[ "$(status "$tmp/pull.json.err")" = 404 ] && gh api "repos/$repo" >/dev/null 2>&1 && { refuse "$pr" pr; return; }
		stop "pr $pr: GitHub's read of the pull request gave $(answer "$tmp/pull.json.err"): once it gives the pull request, pipe this release again"
	fi
	# Only a merge this release makes gets a record, so a pull request merged or closed before the
	# applier's merge call is refused, whoever merged it.
	failed=$(jq -r --slurpfile d "$tmp/d.json" '$d[0].delivery.data as $b | def holds(f): (try f catch false) == true;
		if holds(.base.repo.full_name == $b.repo) | not then "repo"
		elif holds(.number == $b.pr) | not then "pr"
		elif holds(.merged == false and .merged_at == null) | not then "merged"
		elif holds(.state == "open") | not then "closed"
		elif holds(.base.ref == $b.base) | not then "base"
		elif holds(.head.sha == $b.head) | not then "head"
		else "ok" end' "$tmp/pull.json" 2>/dev/null) || failed=pr
	[ "$failed" = ok ] || { refuse "$pr" "$failed"; return; }

	# Kept before the merge call, so a crash after the merge never applies the release twice.
	printf '"%s"\n' "$id" >>"$applied"
	sync
	jq -c --arg m "$method" '{merge_method: $m, sha: .delivery.data.head}' "$tmp/d.json" >"$tmp/merge.in"
	if gh api -X PUT "repos/$repo/pulls/$pr/merge" --input "$tmp/merge.in" >"$tmp/merge.json" 2>"$tmp/merge.err"; then
		answered=yes
	else
		answered=no
		case $(status "$tmp/merge.err") in
		409) refuse "$pr" head-moved; return ;;
		405 | 422) refuse "$pr" merge; return ;;
		401 | 403 | 404 | 429)
			stop "pr $pr: GitHub refused the merge call, $(answer "$tmp/merge.err"), and merged nothing: once GitHub takes the applier's calls again, take its receipt id out of applied.jsonl and pipe this release again"
			;;
		esac
	fi
	# Read again, since a merge call GitHub did not answer may still have merged, and GitHub's merge
	# call fixes the head but not the base: the merge recorded is the one released, at its head into
	# its base.
	read_pull "$tmp/after.json" ||
		stop "pr $pr: the merge call ended and the pull request could not be read again: see by hand whether it merged, and into which base"
	if ! jq -e '.merged == true' "$tmp/after.json" >/dev/null; then
		[ "$answered" = no ] || stop "pr $pr: GitHub answered the merge call, and the pull request read again is not merged: see to it by hand"
		refuse "$pr" merge
		return
	fi
	jq -e --slurpfile d "$tmp/d.json" '$d[0].delivery.data as $b | .base.ref == $b.base and .head.sha == $b.head' "$tmp/after.json" >/dev/null ||
		stop "pr $pr merged into $(jq -r '.base.ref' "$tmp/after.json"), not the base released: no comment or status was posted; see to it by hand"
	commit=$(jq -r '.merge_commit_sha' "$tmp/after.json")
	unrecorded="pr $pr merged, and its record was not posted: post its comment and status by hand from the kept delivery"
	echo "$commit" | grep -Eqx '[0-9a-f]{40}|[0-9a-f]{64}' || stop "$unrecorded"

	# The record: the delivery, one line of JSON, as a code block so that nothing in it renders or
	# notifies anyone, then the status on the merge commit, which names the receipt and links it.
	jq -c '{body: ("    " + (.delivery | tojson))}' "$tmp/d.json" >"$tmp/comment.in"
	gh api -X POST "repos/$repo/issues/$pr/comments" --input "$tmp/comment.in" >"$tmp/comment.json" 2>/dev/null || stop "$unrecorded"
	jq -c --arg id "$id" '{state: "success", context: "placket", description: $id, target_url: .html_url}' "$tmp/comment.json" >"$tmp/status.in"
	gh api -X POST "repos/$repo/statuses/$commit" --input "$tmp/status.in" >/dev/null 2>&1 || stop "$unrecorded"
	echo "applied pr $pr"
}

if [ "$one" = - ]; then
	cat >"$tmp/d.json"
	apply
	exit 0
fi
wait=0
[ "$forever" != yes ] || wait=5
while :; do
	rc=0
	placket get --label "$label" --keep --wait "$wait" --json >"$tmp/d.json" 2>"$tmp/get.err" || rc=$?
	[ "$rc" -eq 0 ] || stop "placket get exited $rc: $(tail -n 1 "$tmp/get.err")"
	if jq -e 'has("delivery")' "$tmp/d.json" >/dev/null; then
		apply
	elif jq -e 'has("close")' "$tmp/d.json" >/dev/null; then
		echo "apply: the Accord is closed" >&2
		exit 0
	elif [ "$forever" != yes ]; then
		exit 0
	fi
done
