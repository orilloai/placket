# Rotating a mediator's key

A mediator's key signs every receipt of its Accord, so it cannot be swapped inside one Accord. To retire a key, close the Accord, and found a new one under a new key whose policy names the close it replaces. Once every member has accepted the new Accord, a verifier checks the old journal against the close the new Accord pins (`spec/protocol.md`, section 13; `docs/adr/0010-the-close-and-the-successor.md`).

This guide uses the files of the worked example's founding steps (`README.md`, "Found an Accord"): the keys `mediator.key`, `principal.key` and each party's, the references `mediator.ref`, `operator.ref` and each party's, the parties' seal entries, each `<party>.seal`, and the openings in `openings/`, `policy.json`, `card.json`, `founding.json` and the `acceptances.json` beside it, and the window's `journal`. It also needs `window.log`, the window's standard error from the start that founded the Accord, which README's window command leaves on the terminal: start that window with `2>window.log` to keep it, or, if the window already runs, write the `accord <id> founded` line it printed, once the last party accepted its view, into `window.log` yourself. Every command runs as written, in this order, in one shell; `tests/test_rotation.sh` runs them all in `make ci`.

## 1. Stop the window, and close its Accord

Stop the running window first, with Ctrl-C. A close runs on the journal of a stopped window: a running window holds the journal's lock, and the close is refused with `journal-locked`.

The close needs the Accord's id, as a guard: a close cannot be undone. Take the id from your own record of the founding, here the line `accord <id> founded` that the window printed when it founded the Accord and that `window.log` keeps, and never from the journal you are closing. If `window.log` holds no such line, `accord` is empty and the close stops with `usage`, writing nothing. An id read from that journal only says that the journal and the founding set agree, which the close checks anyway, so it no longer says which Accord you meant to close.

```sh
accord=$(sed -n 's/^accord \(sha256:[0-9a-f]*\) founded$/\1/p' window.log)
placket window close --key mediator.key --founding founding.json --journal journal --accord "$accord" 2>close.out
close=$(sed -n 's/^close //p' close.out)
```

It closes every question still open, a timeout where its deadline has passed and `closed` otherwise, and writes the close receipt last. Its first line on standard error is `close <id>`. Nothing waits for a member to fetch, unless the close's receipt would pass `MaxReply`: a delivery not yet fetched stays fetchable, and the close lists it. Each delivery not yet fetched makes the receipt longer, so one member that gives parts to a receiver that never fetches can keep the close from being written, and the successor below, which names the close in `supersedes`, waits with it (threat model L22). To hand the close to the members, start the window again on the journal, or run the close with `--socket`: once a member's queue is empty, every `placket get` returns the close and keeps it as `close.json`.

## 2. Found the successor's set under a new key

The successor lives in a directory of its own, `successor/`. A window writes an Accord's `acceptances.json` beside its founding set, so two Accords whose sets share a directory would share that file, and the old Accord's would be read as the new one's.

Make the new key, and a policy that names the old Accord and its close in `supersedes`:

```sh
mkdir successor
placket key new --out successor/mediator.key >successor/mediator.ref
jq -c --arg accord "$accord" --arg close "$close" '. + {supersedes: {accord: $accord, close: $close}}' policy.json >successor/policy.json
echo "{\"actor\":\"$(cat successor/mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >successor/mediator.json
```

A binding names its mediator and its policy, so each party's principal binds it again; the declarations, and their seals, are the same:

```sh
for a in proposer applier decider; do
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat successor/mediator.ref)" --until 2099-01-01T00:00:00Z \
		--policy successor/policy.json >"successor/$a.binding"
done
placket founding --policy successor/policy.json --mediator successor/mediator.json --card card.json \
	--seal proposer.seal --seal applier.seal --seal decider.seal \
	--binding successor/proposer.binding --binding successor/applier.binding --binding successor/decider.binding \
	>successor/founding.json 2>successor/founding.out
successor=$(sed -n 's/^accord //p' successor/founding.out)
```

`placket founding` prints the successor's id, `accord <id>`, on standard error. It is the id each party views and accepts below.

## 3. Run the successor's window until every member has accepted

Start the successor's window with the new key, the same openings and a journal of its own. It starts the new Accord proposed: each party views its own view of it and accepts it, as for the first. The last acceptance founds it, and the window writes `acceptances.json` in `successor/`, beside its founding set; the old Accord's `acceptances.json` is not touched.

```sh
placket window --key successor/mediator.key --founding successor/founding.json --openings openings --socket successor/socket \
	--journal successor/journal 2>successor/window.log &
window=$!
# Wait until the window listens, and stop if it exits first.
until [ -S successor/socket ]; do
	kill -0 "$window"
	sleep 1
done
for a in proposer applier decider; do
	placket view --key "$a.key" --socket successor/socket --accord "$successor"
	placket accept --key "$a.key" --socket successor/socket --accord "$successor"
done
kill "$window"
wait "$window"
```

The window prints `accord <id> founded` once the last party has accepted. Here it is stopped then, so the next step reads a finished `acceptances.json`; start it again the same way, on the same journal, to run the new Accord. Each party keeps the id it accepted: its acceptance, kept in its state directory, names it.

## 4. Check the old journal against the accepted successor

The check takes the successor's founding set, its acceptances and its id. Give the id from your own record, the id you accepted, here `successor`, and never from a file sent with the journal: an id read from the files under check only says that those files agree with each other, as the id of the old Accord would.

```sh
placket verify --receipts journal --accord "$accord" --founding founding.json \
	--superseded-by successor/founding.json --successor-accord "$successor" --successor-acceptances successor/acceptances.json
```

It checks that the successor's set is the one whose typed hash is the id you gave, then founds it with its acceptances at the time of the old Accord's close, so every member must have accepted it and its bindings must outlast the close; the result is the same on whatever day the check runs. Then it checks that the successor's `supersedes` names the old Accord and this journal's close. It exits 0 and prints `complete: against close receipt <close>, which successor accord <new id> pins; not claimed: that the mediator signed no other log for this Accord`.

A successor set that is not the one the id names is tool error `verify superseded`; one that does not found, an acceptance missing or signed for another Accord included, is tool error `founding` with its code. A journal with no close, a successor that supersedes nothing, and one that names another Accord or another close are tool error `verify superseded`. `--superseded-by`, `--successor-accord` and `--successor-acceptances` go together: one or two of them alone are `usage`.

## 5. Retire the old key

Keep the old journal, its `founding.json` and the `acceptances.json` beside it: they are the old Accord's record. Then destroy the old key:

```sh
rm mediator.key
```

Removing the file may not destroy the key. A backup, a copy made when the key was moved, or the file system itself (a journal, a snapshot, a block not yet written over) can keep its bytes, and whoever finds them holds the old key. The key is retired only once every copy is gone: remove each copy and backup too, and treat any you cannot remove as a key that may be stolen (below).

## What this does not protect

Whoever stole the old key before it was destroyed can still sign single receipts for the old Accord, which verify on their own, and a whole other journal with another close. Against a close an accepted successor pins, only the journal that close pins is complete, so check a journal of the old Accord against the successor whose id you accepted, never a receipt alone (`docs/security/threat-model.md`, L19). Rotation within one Accord, and a close the members countersign, are later work.
