#!/bin/sh
# The worked example's test: it runs scripts/demo.sh, the script behind make demo, in a new
# directory, and checks that each of its ten steps printed what the step says, in order, each
# party's check of its own receipts against its view included. Then it
# verifies the demo journal committed under scripts/demo/, the one the README shows a receipt of, as
# any party can with its terms sealed and then with every opening,
# with the binary the run built, and checks that its card names that binary's module. Last, it checks
# the README's receipt: the sentence before it names the committed journal's receipts.jsonl, the
# block is that file's line 1, byte for byte, and the show output under it is what placket show
# --receipt prints for it.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
out=$tmp/out
failed=0

if ! sh "$root/scripts/demo.sh" "$tmp/run" >"$out" 2>&1; then
	echo "FAIL the demo did not run to its end"
	sed 's/^/    | /' "$out"
	exit 1
fi

# The lines each step prints, in order: an extended regular expression a line must match, each one
# after the line the one before it matched.
want='^== 1\.
^card sha256:[0-9a-f]{64}$
^module 0\.2\.0 sha256:[0-9a-f]{64}$
^accord sha256:[0-9a-f]{64}$
^== 2\.
^window: accord sha256:[0-9a-f]{64} proposed$
^window: card sha256:[0-9a-f]{64}$
^window: operator ed25519:[0-9a-f]{64}$
^proposer: give proposal to ed25519:[0-9a-f]{64} \(applier\): ask, answered by role:decider within 3600 seconds$
^proposer: accepted view sha256:[0-9a-f]{64}$
^proposer: the Accord is not founded yet: it waits for every member.s acceptance$
^decider: answer proposal from ed25519:[0-9a-f]{64} to ed25519:[0-9a-f]{64} within 3600 seconds$
^decider: the Accord is founded: every member accepted its view$
^window: accord sha256:[0-9a-f]{64} founded$
^== 3 and 4\.
^decider: questions: Rename the column total to amount\.$
^decider: pass ok sha256:
^decider: released ok sha256:
^decider: exit 0$
^proposer: ask maybe sha256:
^proposer: accepted sha256:
^proposer: exit 0$
^applier: delivery sha256:[0-9a-f]{64} kind proposal 
^applier: exit 0$
^applier: Rename the column total to amount\.$
^== 5\.
^decider: pass ok sha256:
^decider: released ok sha256:
^decider: pass ok sha256:
^decider: exit 0$
^proposer: changed sha256:
^proposer: Keep the column notes, but hide it\.$
^proposer: exit 1$
^== 6\.
^decider: block declined sha256:
^decider: pass ok sha256:
^decider: exit 0$
^proposer: declined sha256:
^proposer: exit 1$
^== 7\.
^proposer: ask maybe sha256:
^proposer: exit 4$
^proposer: rejected not-an-answerer$
^proposer: exit 2$
^== 8\.
^proposer: block no-match sha256:
^proposer: exit 1$
^== 9\.
^window: reloaded block undeliverable sha256:
^window: accord sha256:[0-9a-f]{64} reloaded$
^proposer: undeliverable sha256:
^proposer: exit 1$
^applier: delivery sha256:[0-9a-f]{64} kind changed-version 
^applier: Keep the column notes, but hide it\.$
^proposer: Someone still reads it\.$
^proposer: Invoices are kept for seven years\.$
^proposer: no delivery$
^applier: no delivery$
^decider: no delivery$
^== 10\.
^verifier: operator ed25519:[0-9a-f]{64} runs process$
^verifier: module 0\.2\.0 sha256:
^verifier: member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$
^verifier: member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$
^verifier: member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$
^verifier: terms: not checked: the declarations are sealed
^verifier: pass 6 block 3 ask 4$
^verifier: questions closed 4 open 0$
^verifier: releases 2 blocked releases 0$
^verifier: sha256:[0-9a-f]{64} witnessed: signed by key ed25519:[0-9a-f]{64}, which principal ed25519:[0-9a-f]{64} bound to role decider$
^verifier: valid: 13 receipts signed by mediator ed25519:[0-9a-f]{64}$
^verifier: conforms: every receipt names this Accord, its card and its members
^verifier: complete: not claimed: this Accord has no close receipt$
^verifier: claim: each receipt is this mediator.s signed decision about one part under this Accord.s terms\.$
^verifier: not claimed: what any party did with a part after it crossed; 
^operator: seal ed25519:[0-9a-f]{64}: opens: the declaration it seals is its actor.s own$
^operator: view ed25519:[0-9a-f]{64}: matches its acceptance$
^operator: terms: checked: every seal opens, and every view is the one its member accepted$
^operator: valid: 13 receipts signed by mediator ed25519:[0-9a-f]{64}$
^operator: conforms: every decision is one this Accord let its decider make$
^proposer: view of ed25519:[0-9a-f]{64}: receipts\.jsonl line 1 receipt sha256:[0-9a-f]{64} matches give proposal to ed25519:[0-9a-f]{64}: ask$
^proposer: view of ed25519:[0-9a-f]{64}: every receipt of its own, 13, matches the view it accepted$
^proposer: exit 0$
^applier: view of ed25519:[0-9a-f]{64}: every receipt of its own, 11, matches the view it accepted$
^applier: exit 0$
^decider: view of ed25519:[0-9a-f]{64}: receipts\.jsonl line 7 receipt sha256:[0-9a-f]{64} matches give comment to ed25519:[0-9a-f]{64}: pass$
^decider: view of ed25519:[0-9a-f]{64}: every receipt of its own, 2, matches the view it accepted$
^decider: exit 0$
^verifier: nothing to reconcile$
^verifier: proposal,topic,kind,asked_at,answered_at,answer,released_at,comment,answer_signed,answerer,receipts$
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,accepted,[0-9TZ:-]+,,yes,
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,changed,[0-9TZ:-]+,Someone still reads it\.,yes,
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,declined,,Invoices are kept for seven years\.,yes,
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,undeliverable,,,,
^verifier: error verify signature receipts\.jsonl line 1$
^verifier: exit 3$
^verifier: error verify signature receipts\.jsonl line 1$
^verifier: exit 3$
^demo: done'
printf '%s\n' "$want" >"$tmp/want"
missing=$(awk 'NR == FNR { w[++n] = $0; next } i < n && $0 ~ w[i + 1] { i++ } END { if (i < n) print w[i + 1] }' "$tmp/want" "$out")
if [ -n "$missing" ]; then
	echo "FAIL the demo printed no line matching $missing where it should, in this order:"
	sed 's/^/    | /' "$out"
	failed=1
else
	echo "ok   the demo's ten steps print what each step says"
fi
if [ "$(grep -Ec '^verifier: sha256:[0-9a-f]{64},' "$out")" -ne 4 ]; then
	echo "FAIL the log has $(grep -Ec '^verifier: sha256:[0-9a-f]{64},' "$out") rows, not 4"
	failed=1
fi

demo=$root/scripts/demo
accord=$(sed -n 's/^{"accord":"\(sha256:[0-9a-f]*\)".*/\1/p' "$demo/journal/accord")
if "$tmp/run/placket" verify --accord "$accord" --receipts "$demo/journal" --founding "$demo/founding.json" >"$tmp/verify" 2>&1 &&
	grep -q '^valid: ' "$tmp/verify" && grep -q '^terms: not checked: the declarations are sealed' "$tmp/verify"; then
	echo "ok   the committed demo journal verifies, its terms sealed"
	opens=""
	for o in "$demo"/openings/*.opening.json; do opens="$opens --open $o"; done
	# With every opening, its terms are checked, the journal conforms, and its answers.jsonl witnesses
	# the accept.
	# shellcheck disable=SC2086
	"$tmp/run/placket" verify --accord "$accord" --receipts "$demo/journal" --founding "$demo/founding.json" $opens >"$tmp/verify" 2>&1 || :
	if grep -qx 'conforms: every decision is one this Accord let its decider make' "$tmp/verify" &&
		grep -qx 'terms: checked: every seal opens, and every view is the one its member accepted' "$tmp/verify" &&
		grep -qE '^sha256:[0-9a-f]{64} witnessed: signed by key ed25519:[0-9a-f]{64}, which principal ed25519:[0-9a-f]{64} bound to role decider$' "$tmp/verify"; then
		echo "ok   the committed demo journal conforms, its accept witnessed"
	else
		echo "FAIL the committed demo journal does not conform with its accept witnessed"
		sed 's/^/    | /' "$tmp/verify"
		failed=1
	fi
	# One principal stands behind all three members: three member lines, one principal among them.
	if [ "$(grep -cE '^member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$' "$tmp/verify")" -eq 3 ] &&
		[ "$(awk '$1 == "member" { print $6 }' "$tmp/verify" | sort -u | wc -l)" -eq 1 ]; then
		echo "ok   the committed demo journal's members have one principal"
	else
		echo "FAIL the committed demo journal's members are not three under one principal"
		sed 's/^/    | /' "$tmp/verify"
		failed=1
	fi
	# The README's command names no other build's module: the committed journal is founded under the
	# card this build prints. A change to the kernel hash or a frozen format writes the journal again,
	# and quotes the README's first receipt again, in its own change.
	if grep -q "is not this build's decision module" "$tmp/verify"; then
		echo "FAIL the committed demo journal's card names another build's module"
		sed 's/^/    | /' "$tmp/verify"
		failed=1
	else
		echo "ok   the committed demo journal's card names this build's module"
	fi
else
	echo "FAIL the committed demo journal does not verify"
	sed 's/^/    | /' "$tmp/verify"
	failed=1
fi

# The README's receipt is the committed journal's first, named so in the sentence before it, and the
# show output under it is what placket show --receipt prints for that receipt, standard error first.
# blocks: each code block of the README, its lines, after the line "@@ <the sentence before it>",
# the last line of text above its fence.
awk '/^```/ { if (!fence) print "@@ " last; fence = !fence; next } fence { print; next } NF { last = $0 }' "$root/README.md" >"$tmp/blocks"
sentence=$(grep -B 1 -x -F "$(head -n 1 "$demo/journal/receipts.jsonl")" "$tmp/blocks" | sed -n 's/^@@ //p')
first=$(grep -n -x -F "$(head -n 1 "$demo/journal/receipts.jsonl")" "$tmp/blocks" | head -n 1 | cut -d: -f1)
case $sentence in
*'scripts/demo/journal/receipts.jsonl'*) echo "ok   the sentence before the README's receipt names scripts/demo/journal/receipts.jsonl" ;;
*)
	echo "FAIL the sentence before the README's receipt does not name where it comes from, scripts/demo/journal/receipts.jsonl"
	echo "    | ${sentence:-no block of the README is line 1 of the committed journal}"
	failed=1
	;;
esac
if [ -n "$first" ] && [ "$(sed -n "$((first - 1))p;$((first + 1))p" "$tmp/blocks" | sed -n 's/^@@ .*/@@/p' | wc -l)" -eq 2 ]; then
	echo "ok   the README's receipt is line 1 of scripts/demo/journal/receipts.jsonl, byte for byte"
else
	echo "FAIL no code block of the README is line 1 of scripts/demo/journal/receipts.jsonl, alone and byte for byte"
	failed=1
fi
head -n 1 "$demo/journal/receipts.jsonl" >"$tmp/receipt.json"
"$tmp/run/placket" show --receipt "$tmp/receipt.json" >"$tmp/show.out" 2>"$tmp/show.err"
cat "$tmp/show.err" "$tmp/show.out" >"$tmp/show"
# The block after the receipt's.
awk -v n="${first:-0}" 'NR > n && /^@@ / { b++; next } NR > n && b == 1 { print } b > 1 { exit }' "$tmp/blocks" >"$tmp/shown"
if [ -n "$first" ] && [ -s "$tmp/show" ] && cmp -s "$tmp/show" "$tmp/shown"; then
	echo "ok   the README's show output is what placket show --receipt prints for that receipt"
else
	echo "FAIL the README's show output is not what placket show --receipt prints for line 1 of the committed journal"
	diff "$tmp/show" "$tmp/shown" | sed 's/^/    > /'
	failed=1
fi
[ "$failed" -eq 0 ]
