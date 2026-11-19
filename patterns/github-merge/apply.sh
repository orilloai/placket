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
# `placket get --label LABEL --keep --wait 5 --json` and handles it. With `-`, it handles the one
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
#   read        GitHub did not answer the read of the pull request, for a reason other than that,
#               or than a refusal of the applier's credential, HTTP 401, which stops the applier
#               instead, so that a lapsed credential does not refuse every release after it;
#   merged      the pull request is not merged already, by anyone, before the applier's merge call;
#   closed      it is open;
#   base, head  its base branch and head commit are the body's;
#   head-moved  GitHub merges it with sha set to the body's head, which it refuses if the head moved;
#   merge       GitHub refuses the merge for any other reason but HTTP 401, such as a ruleset, or it
#               gives no answer and the pull request read again is not merged.
# Its receipt id goes into applied.jsonl, synced, before the merge call. Once merged, the applier
# reads the pull request again, and stops, for a person, unless it merged at the body's head into
# the body's base. Then the delivery is posted as one comment on the pull request, as a code block so
# that nothing in it renders, and a commit status on the merge commit, context placket, names the
# receipt id and links that comment.
set -eu

role=decider # CHANGE: the role whose accept releases a merge, as your Accord's terms name it
method=merge # CHANGE: merge, squash or rebase, as the repository allows
forever=no   # CHANGE: yes, to wait for releases until interrupted; no stops once the queue is empty

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
	n=$(jq -r '.delivery.data | if type == "object" then .pr else null end
		| if type == "number" and . >= 1 and . == floor and . < 1e9 then floor | tostring else "-" end' "$d" 2>/dev/null) || n=-
	[ -n "$n" ] || n=-
	if ! jq -e --argjson most "$most" --arg ref "$ref" --arg bad "$bad" '.delivery | .kind == "merge" and .text == null and .raw == null
		and (.receiptId | type == "string" and test("^sha256:[0-9a-f]{64}$"))
		and (.messageId | type == "string" and test("^[A-Za-z0-9._:-]{1,128}$"))
		and (.re == null or (.re | type == "string" and test("^sha256:[0-9a-f]{64}$")))
		and (tojson | length <= $most)
		and (.data | type == "object" and keys == ["base", "head", "pr", "repo"])
		and (.data.repo | type == "string" and test("^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$"))
		and (.data.pr | type == "number" and . >= 1 and . == floor and . < 1e9)
		and (.data.base | type == "string" and length <= 255 and test($ref) and (test($bad) | not))
		and (.data.head | type == "string" and test("^([0-9a-f]{40}|[0-9a-f]{64})$"))' "$d" >/dev/null 2>&1; then
		refuse "$n" body
		return
	fi
	# placket get has refused a delivery whose answer call is not the one its receipt names; the
	# call's signature is the mediator's to check (README.md, "What it claims").
	if ! jq -e --slurpfile keys "$tmp/deciders.json" '.delivery.answer | type == "object"
		and .action == "accepted" and (.from | type == "string") and (.from as $f | $keys[0] | any(. == $f))' "$d" >/dev/null; then
		refuse "$n" unwitnessed
		return
	fi
	id=$(jq -r '.delivery.receiptId' "$d")
	if grep -Fqx "\"$id\"" "$applied"; then
		refuse "$n" applied
		return
	fi
	[ "$(jq -r '.delivery.data.repo' "$d")" = "$repo" ] || { refuse "$n" repo; return; }
	pr=$n
	body=$(jq -c '.delivery.data' "$d")
	head=$(jq -r '.delivery.data.head' "$d")

	if ! gh api "repos/$repo/pulls/$pr" >"$tmp/pull.json" 2>"$tmp/pull.err"; then
		case $(code "$tmp/pull.err") in
		404) refuse "$pr" pr ;;
		401) lapsed "$pr" "the read" "pipe this release again" ;;
		*) refuse "$pr" read ;;
		esac
		return
	fi
	# Only a merge this release makes gets a record: a pull request merged, or closed, before the
	# merge call is refused, whoever merged it.
	for check in \
		"pr|.number == (\$b.pr)" \
		"repo|(.base.repo.full_name | ascii_downcase) == (\$b.repo | ascii_downcase)" \
		"merged|.merged == false and .merged_at == null" \
		"closed|.state == \"open\"" \
		"base|.base.ref == \$b.base" \
		"head|.head.sha == \$b.head"; do
		if ! jq -e --argjson b "$body" "${check#*|}" "$tmp/pull.json" >/dev/null 2>&1; then
			refuse "$pr" "${check%%|*}"
			return
		fi
	done

	# Kept before the merge call, so a crash after the merge never applies the release twice.
	jq -nc --arg id "$id" '$id' >>"$applied"
	sync "$applied" 2>/dev/null || sync
	merged=
	if jq -nc --arg method "$method" --arg sha "$head" '{merge_method: $method, sha: $sha}' |
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
	jq -e --argjson b "$body" '.base.ref == $b.base and .head.sha == $b.head' "$tmp/after.json" >/dev/null 2>&1 ||
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
	placket get --label "$label" --keep --wait 5 --json >"$tmp/get.json" 2>/dev/null || rc=$?
	if [ "$rc" -ne 0 ]; then
		echo "apply: placket get exited $rc: $(jq -r '.error // empty' "$tmp/get.json" 2>/dev/null)" >&2
		exit 1
	fi
	if jq -e 'has("delivery")' "$tmp/get.json" >/dev/null; then
		apply "$tmp/get.json"
		continue
	fi
	if jq -e 'has("close")' "$tmp/get.json" >/dev/null; then
		echo "apply: the Accord is closed" >&2
		break
	fi
	[ "$forever" = yes ] || break
done
