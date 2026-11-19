#!/bin/sh
# The local applier. It writes a file only with what Placket released to it: the text a person in
# the deciding role accepted, or the version they changed it to. A declined proposal releases
# nothing, so it writes nothing. Copy this file alone to run it for real; pattern.sh runs it on
# invented data. See README.md.
#
#   sh apply.sh LABEL FILE
#
# LABEL names the applier's mediator in the labels config, which placket finds where the caller's
# XDG_CONFIG_HOME, or its default, says; FILE is the file it writes. It sets nothing up. It fetches
# each release with `placket get --label LABEL --keep --json` and handles it, and stops once the queue
# is empty. It writes each release's text to FILE in place of what FILE held, through a new file
# beside it, mode 0600, renamed over it, and prints `applied accepted FILE` or `applied changed FILE`; or it prints
# `refused <reason>` and writes nothing. The reasons, in the order it checks them:
#   body         the delivery is not a text of kind proposal or changed-version;
#   unwitnessed  it carries no answer call, or the call is not an accept of a proposal or a change of
#                a changed version, or its from is not a key the founding set's bindings give the
#                deciding role.
set -eu

role=decider # CHANGE: the role whose answer releases a proposal, as your Accord's terms name it
forever=no   # CHANGE: yes, to wait for releases until interrupted; no stops once the queue is empty

[ $# -eq 2 ] || { echo "usage: sh apply.sh LABEL FILE" >&2; exit 2; }
label=$1 file=$2

tmp=$(mktemp -d "${TMPDIR:-/tmp}/apply.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

# The keys the founding set's bindings give the deciding role. The terms that name the role are
# sealed; the bindings are in the clear.
founding=$(placket mediators --json | jq -r --arg l "$label" '.mediators[] | select(.label == $l) | .founding')
[ -n "$founding" ] || { echo "apply: the labels config has no mediator labelled $label" >&2; exit 2; }
jq -c --arg role "$role" '[.bindings[] | select(.roles | index($role)) | .actor]' "$founding" >"$tmp/deciders.json"

# apply: handles the one delivery in $tmp/d.json. placket get has refused a delivery whose answer
# call is not the one its receipt names.
apply() {
	verdict=$(jq -r --slurpfile keys "$tmp/deciders.json" '.delivery as $d | def holds(f): (try f catch false) == true;
		if holds(($d.kind == "proposal" or $d.kind == "changed-version") and ($d.text | type == "string")) | not then "body"
		elif holds(($d.answer.from as $f | $keys[0] | any(.[]; . == $f))
			and (($d.kind == "proposal" and $d.answer.action == "accepted") or ($d.kind == "changed-version" and $d.answer.action == "changed"))) | not
			then "unwitnessed"
		else $d.answer.action end' "$tmp/d.json")
	case $verdict in
	accepted | changed) ;;
	*) echo "refused $verdict"; return ;;
	esac
	# mktemp makes the file beside FILE under a name nobody can guess, and fails if anything is there.
	part=$(mktemp "$file.XXXXXX")
	jq -j '.delivery.text' "$tmp/d.json" >"$part" && mv "$part" "$file" || { rm -f "$part"; exit 1; }
	echo "applied $verdict $file"
}

wait=0
[ "$forever" != yes ] || wait=5
while :; do
	rc=0
	placket get --label "$label" --keep --wait "$wait" --json >"$tmp/d.json" 2>"$tmp/get.err" || rc=$?
	[ "$rc" -eq 0 ] || { echo "apply: placket get exited $rc: $(tail -n 1 "$tmp/get.err")" >&2; exit 1; }
	case $(jq -r 'if has("delivery") then "delivery" elif has("close") then "close" else "none" end' "$tmp/d.json") in
	delivery) apply ;;
	close) echo "apply: the Accord is closed" >&2; exit 0 ;;
	*) [ "$forever" = yes ] || exit 0 ;;
	esac
done
