#!/bin/sh
# The worked example's test: it runs scripts/demo.sh, the script behind make demo, in a new
# directory, and checks that each of its ten steps printed what the step says, in order. Then it
# verifies the demo journal committed under scripts/demo/, the one the README shows a receipt of,
# with the binary the run built, and checks that its card names that binary's module.
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
^module 0\.1\.0 sha256:[0-9a-f]{64}$
^accord sha256:[0-9a-f]{64}$
^== 2\.
^window: accord sha256:[0-9a-f]{64} founded$
^window: card sha256:[0-9a-f]{64}$
^window: operator ed25519:[0-9a-f]{64}$
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
^verifier: verified 13 receipts$
^verifier: operator ed25519:[0-9a-f]{64} runs process$
^verifier: module 0\.1\.0 sha256:
^verifier: member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$
^verifier: member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$
^verifier: member ed25519:[0-9a-f]{64} roles (proposer|applier|decider) principal ed25519:[0-9a-f]{64}$
^verifier: pass 6 block 3 ask 4$
^verifier: questions closed 4 open 0$
^verifier: releases 2 blocked releases 0$
^verifier: claim: each receipt is this mediator.s signed decision about one part under this Accord.s terms\.$
^verifier: not claimed: what any party did with a part after it crossed; 
^verifier: nothing to reconcile$
^verifier: proposal,topic,kind,asked_at,answered_at,answer,released_at,comment,answer_signed,answerer,receipts$
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,accepted,[0-9TZ:-]+,,yes,
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,changed,[0-9TZ:-]+,Someone still reads it\.,yes,
^verifier: sha256:[0-9a-f]{64},billing,proposal,[^,]*,[^,]*,declined,,Invoices are kept for seven years\.,,
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
if "$tmp/run/placket" verify --mediator "$(cat "$demo/mediator.ref")" --receipts "$demo/journal" --founding "$demo/founding.json" >"$tmp/verify" 2>&1; then
	echo "ok   the committed demo journal verifies"
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
[ "$failed" -eq 0 ]
