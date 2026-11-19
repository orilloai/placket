#!/bin/sh
# The GitHub merge applier: it merges a pull request only when Placket released a merge of it, a key
# bound to the deciding role accepted that release, and the pull request is still what was released,
# at the head commit released. Copy this file alone to run it for real; pattern.sh runs it on
# invented data. See README.md, and custody.md for the GitHub settings it rests on.
#
#   sh apply.sh LABEL OWNER/NAME [-]
#
# LABEL is the applier's mediator in the labels config; OWNER/NAME is the one repository it serves.
# It reads the caller's environment and sets nothing up: placket finds the labels config where the
# caller's XDG_CONFIG_HOME, or its default, says. It fetches each delivery with
# `placket get --label LABEL --keep --json` and handles it, and stops at the first fetch that finds
# the queue empty; with forever set to yes, each fetch waits up to 5 seconds for a release, with
# `--wait 5`, and an empty one starts the next. With `-`, it handles the one
# delivery on standard input, as `placket get --json` printed it, and fetches nothing; placket checks
# none of that input, so feed it only a line of the applier's own kept.jsonl (README.md, "When it
# stops partway"). For each delivery it prints one line, `applied pr <n>` or `refused pr <n> <reason>`,
# `-` standing for <n> when the body names no number. A refused release is never tried again.
#
# A release is applied only when, in this order:
#   body        the delivery's body is data of kind merge with exactly repo, pr, base and head, its
#               message id is a short plain id, and the whole delivery fits in one comment;
#   unwitnessed it carries the answer call that released it, an accept by a key the founding set's
#               bindings give the deciding role;
#   applied     its receipt id is not in applied.jsonl, beside this file;
#   repo        the body's repo is OWNER/NAME, and GitHub's pull request is in it;
#   pr          GitHub has the pull request the body names;
#   merged      the pull request is not merged already, by anyone, before the applier's merge call;
#   closed      it is open;
#   base, head  its base branch and head commit are the body's;
#   head-moved  GitHub merges it with sha set to the body's head, which it refuses if the head moved;
#   merge       GitHub refuses the merge for any other reason but HTTP 401, such as a ruleset, or it
#               gives no answer and the pull request read again is not merged.
# Any other failure to read the pull request, such as a refused credential, HTTP 401, GitHub
# unavailable, or no answer at all, stops the applier, for a person, so that an outage or a lapsed
# credential does not refuse every release queued after it.
# Its receipt id goes into applied.jsonl, synced, before the merge call. Once merged, the applier
# reads the pull request again, and stops, for a person, unless it merged at the body's head into
# the body's base. Then the delivery is posted as one comment on the pull request, as a code block so
# that nothing in it renders, and a commit status on the merge commit, context placket, names the
# receipt id and links that comment.
set -eu

role=decider # CHANGE: the role whose accept releases a merge, as your Accord's terms name it
method=merge # CHANGE: merge, squash or rebase, as the repository allows
forever=no   # CHANGE: yes, to wait for releases until interrupted; no stops once the queue is empty
# As it ships, the applier takes only the releases already queued, and waits for none.
patience=0
[ "$forever" != yes ] || patience=5

usage() {
	echo "usage: sh apply.sh LABEL OWNER/NAME [-]" >&2
	exit 2
}
[ $# -eq 2 ] || { [ $# -eq 3 ] && [ "$3" = - ]; } || usage
label=$1 repo=$2
one=${3:-}
# The repository is a path of GitHub's API: an owner and a name, each of letters, digits, '.', '_'
# and '-', and neither of them '.' or '..'.
case $repo in
*/*/* | */. | */.. | ./* | ../* | /* | */ | *[!A-Za-z0-9._/-]*) usage ;;
*/*) ;;
*) usage ;;
esac

here=$(cd "$(dirname "$0")" && pwd)
applied=$here/applied.jsonl
tmp=$(mktemp -d "${TMPDIR:-/tmp}/apply.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

# The keys the founding set's bindings give the deciding role. The terms that name the role are
# sealed, but the bindings are in the clear.
placket mediators --json >"$tmp/mediators.json"
founding=$(jq -r --arg l "$label" '.mediators[] | select(.label == $l) | .founding' "$tmp/mediators.json")
[ -n "$founding" ] || { echo "apply: the labels config has no mediator labelled $label" >&2; exit 2; }
jq -c --arg role "$role" '[.bindings[] | select(.roles | any(. == $role)) | .actor]' "$founding" >"$tmp/deciders.json"
touch "$applied"

# refuse N REASON: the release of pull request N is not applied, for REASON.
refuse() { echo "refused pr $1 $2"; }

# stop LINE: the applier stops, for a person to look at what LINE says (README.md, "When it stops
# partway").
stop() {
	echo "apply: $1" >&2
	exit 1
}

# code FILE: the HTTP status of the error gh printed into FILE, or nothing.
code() { grep -o '(HTTP [0-9][0-9]*)' "$1" | tail -n 1 | tr -dc 0-9 || :; }

# lapsed N WHAT THEN: GitHub refused the applier's credential at WHAT, for pull request N; the
# applier stops, for a person to renew it (custody.md, step 4) and then do THEN.
lapsed() { stop "pr $1: GitHub refused the applier's credential, HTTP 401, at $2: renew it, then $3 (README.md, \"When it stops partway\")"; }

# A branch's name as git takes one (git check-ref-format): no space, control character, '~', '^',
# ':', '?', '*', '[' or '\'; no '..', '//' or '@{'; no part that starts with '.' or ends with '.lock';
# and neither '/' nor '.' at its end, nor '/' at its start, nor '@' alone.
ref='^[^\x00-\x20\x7f~^:?*\[\\]+$'
bad='\.\.|//|@\{|(^|/)\.|\.lock(/|$)|[/.]$|^/|^@$'

# The longest comment GitHub takes, less the code block's indent.
most=65532

# apply FILE: handles the one delivery FILE holds, as placket get --json printed it.
apply() {
	d=$1
	# One read of the delivery, which FILE holds alone, gives the pull request's number, or -, the
	# first of the checks body, unwitnessed, applied and repo that it fails, or ok, and its receipt id. placket get has refused a
	# delivery whose answer call is not the one its receipt names; the call's signature is the
	# mediator's to check (README.md, "What it claims").
	jq -r --argjson most "$most" --arg ref "$ref" --arg bad "$bad" --arg repo "$repo" \
		--slurpfile keys "$tmp/deciders.json" --rawfile done "$applied" -s 'if length != 1 then "- body -" else .[0].delivery
		| (.data | if type == "object" then .pr else null end
			| if type == "number" and . >= 1 and . == floor and . < 1e9 then floor | tostring else "-" end) as $n
		| if (try (.kind == "merge" and .text == null and .raw == null
			and (.receiptId | type == "string" and test("^sha256:[0-9a-f]{64}$"))
			and (.messageId | type == "string" and test("^[A-Za-z0-9._:-]{1,128}$"))
			and (.re == null or (.re | type == "string" and test("^sha256:[0-9a-f]{64}$")))
			and (tojson | length <= $most)
			and (.data | type == "object" and keys == ["base", "head", "pr", "repo"])
			and (.data.repo | type == "string" and test("^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$"))
			and (.data.pr | type == "number" and . >= 1 and . == floor and . < 1e9)
			and (.data.base | type == "string" and length <= 255 and test($ref) and (test($bad) | not))
			and (.data.head | type == "string" and test("^([0-9a-f]{40}|[0-9a-f]{64})$"))) catch false) != true
		then "\($n) body -"
		elif (try (.answer | type == "object" and .action == "accepted" and (.from | type == "string")
			and (.from as $f | $keys[0] | any(. == $f))) catch false) != true
		then "\($n) unwitnessed -"
		elif ("\"" + .receiptId + "\"") as $line | $done | split("\n") | any(. == $line)
		then "\($n) applied -"
		elif .data.repo != $repo then "\($n) repo -"
		else "\($n) ok \(.receiptId)" end end' "$d" >"$tmp/verdict" 2>/dev/null || echo "- body -" >"$tmp/verdict"
	read -r n verdict id <"$tmp/verdict" || :
	[ -n "${n:-}" ] || n=-
	case ${verdict:-} in
	ok) ;;
	unwitnessed | applied | repo) refuse "$n" "$verdict"; return ;;
	*) refuse "$n" body; return ;;
	esac
	pr=$n

	if ! gh api "repos/$repo/pulls/$pr" >"$tmp/pull.json" 2>"$tmp/pull.err"; then
		case $(code "$tmp/pull.err") in
		404) refuse "$pr" pr ;;
		401) lapsed "$pr" "the read" "pipe this release again" ;;
		*) stop "pr $pr: GitHub did not answer the read of the pull request: once it does, pipe this release again (README.md, \"When it stops partway\")" ;;
		esac
		return
	fi
	# Only a merge this release makes gets a record: a pull request merged, or closed, before the
	# merge call is refused, whoever merged it. The first check it fails, or ok.
	failed=$(jq -r --slurpfile d "$d" '$d[0].delivery.data as $b
		| def holds(f): (try f catch false) == true;
		if holds(.number == $b.pr) | not then "pr"
		elif holds((.base.repo.full_name | ascii_downcase) == ($b.repo | ascii_downcase)) | not then "repo"
		elif holds(.merged == false and .merged_at == null) | not then "merged"
		elif holds(.state == "open") | not then "closed"
		elif holds(.base.ref == $b.base) | not then "base"
		elif holds(.head.sha == $b.head) | not then "head"
		else "ok" end' "$tmp/pull.json" 2>/dev/null) || failed=pr
	[ "$failed" = ok ] || { refuse "$pr" "$failed"; return; }

	# Kept before the merge call, so a crash after the merge never applies the release twice.
	printf '"%s"\n' "$id" >>"$applied"
	sync "$applied" 2>/dev/null || sync
	merged=
	if jq -c --arg method "$method" '{merge_method: $method, sha: .delivery.data.head}' "$d" |
		gh api -X PUT "repos/$repo/pulls/$pr/merge" --input - >"$tmp/merge.json" 2>"$tmp/merge.err"; then
		merged=$(jq -r '.sha // empty' "$tmp/merge.json" 2>/dev/null) || merged=
	else
		# GitHub's own refusal, HTTP 4xx, merged nothing: whatever the pull request is now, this
		# release did not merge it.
		case $(code "$tmp/merge.err") in
		401) lapsed "$pr" "the merge call, which merged nothing" "remove its receipt id's line from applied.jsonl and pipe it again" ;;
		409) refuse "$pr" head-moved; return ;;
		4??) refuse "$pr" merge; return ;;
		esac
	fi
	# Read again: a merge call with no answer, or a server's error, may still have merged, and the
	# merge must be the one released, at its head into its base.
	gh api "repos/$repo/pulls/$pr" >"$tmp/after.json" 2>/dev/null ||
		stop "pr $pr: the merge call ended and the pull request could not be read again: check by hand whether it merged, and into which base"
	if ! jq -e '.merged == true' "$tmp/after.json" >/dev/null 2>&1; then
		[ -z "$merged" ] || stop "pr $pr: GitHub answered the merge call, and the pull request read again is not merged: check it by hand"
		refuse "$pr" merge
		return
	fi
	into=$(jq -r '.base.ref' "$tmp/after.json")
	jq -e --slurpfile d "$d" '$d[0].delivery.data as $b | .base.ref == $b.base and .head.sha == $b.head' "$tmp/after.json" >/dev/null 2>&1 ||
		stop "pr $pr merged into $into, not into the base released: a person must handle it; no comment or status was posted"
	[ -n "$merged" ] || merged=$(jq -r '.merge_commit_sha // empty' "$tmp/after.json")
	case $merged in
	*[!0-9a-f]* | '') unrecorded "$pr" ;;
	esac
	jq -c '{body: ("    " + (.delivery | tojson))}' "$d" |
		gh api -X POST "repos/$repo/issues/$pr/comments" --input - >"$tmp/comment.json" 2>/dev/null || unrecorded "$pr"
	url=$(jq -r '.html_url' "$tmp/comment.json")
	jq -nc --arg id "$id" --arg url "$url" '{state: "success", context: "placket", description: $id, target_url: $url}' |
		gh api -X POST "repos/$repo/statuses/$merged" --input - >/dev/null 2>&1 || unrecorded "$pr"
	echo "applied pr $pr"
}

# unrecorded N: pull request N merged, and its comment or its status was not posted; the applier
# stops, for a person to post them by hand from the kept delivery (README.md).
unrecorded() { stop "merged pr $1, but its comment or status was not posted: post them by hand (README.md)"; }

if [ "$one" = - ]; then
	cat >"$tmp/delivery.json"
	apply "$tmp/delivery.json"
	exit 0
fi
while :; do
	rc=0
	placket get --label "$label" --keep --wait "$patience" --json >"$tmp/get.json" 2>/dev/null || rc=$?
	if [ "$rc" -ne 0 ]; then
		echo "apply: placket get exited $rc: $(jq -r '.error // empty' "$tmp/get.json" 2>/dev/null)" >&2
		exit 1
	fi
	got=$(jq -r 'if has("delivery") then "delivery" elif has("close") then "close" else "none" end' "$tmp/get.json")
	if [ "$got" = delivery ]; then
		apply "$tmp/get.json"
		continue
	fi
	if [ "$got" = close ]; then
		echo "apply: the Accord is closed" >&2
		break
	fi
	[ "$forever" = yes ] || break
done
