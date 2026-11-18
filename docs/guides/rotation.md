# Rotating a mediator's key

A mediator's key signs every receipt of its Accord, so it cannot be swapped inside one Accord. To retire a key, close the Accord, and found a new one under a new key whose policy names the close it replaces. A verifier then checks the old journal against the close the new Accord pins (`spec/protocol.md`, section 13; `docs/adr/0022-the-close.md`).

This guide uses the files of the worked example's founding steps (`README.md`, "Found an Accord"): the keys `mediator.key` and `principal.key`, the references `mediator.ref`, `operator.ref` and each party's, the parties' seal entries, each `<party>.seal`, and the openings in `openings/`, `policy.json`, `card.json`, `founding.json` and the `acceptances.json` beside it, and the window's `journal`. It also needs `window.log`, the window's standard error from the start that founded the Accord, which README's window command leaves on the terminal: start that window with `2>window.log` to keep it, or, if the window already runs, write the `accord <id> founded` line it printed, once the last party accepted its view, into `window.log` yourself. Every command but the last two runs as written; `tests/test_rotation.sh` runs them in `make ci`.

## 1. Stop the window, and close its Accord

Stop the running window first, with Ctrl-C. A close runs on the journal of a stopped window: a running window holds the journal's lock, and the close is refused with `journal-locked`.

The close needs the Accord's id, as a guard: a close cannot be undone. Take the id from your own record of the founding, here the line `accord <id> founded` that the window printed when it founded the Accord and that `window.log` keeps, and never from the journal you are closing. If `window.log` holds no such line, `accord` is empty and the close stops with `usage`, writing nothing. An id read from that journal only says that the journal and the founding set agree, which the close checks anyway, so it no longer says which Accord you meant to close.

```sh
accord=$(sed -n 's/^accord \(sha256:[0-9a-f]*\) founded$/\1/p' window.log)
placket window close --key mediator.key --founding founding.json --journal journal --accord "$accord" 2>close.out
close=$(sed -n 's/^close //p' close.out)
```

It closes every question still open, a timeout where its deadline has passed and `closed` otherwise, and writes the close receipt last. Its first line on standard error is `close <id>`. Nothing waits for a member to fetch: a delivery not yet fetched stays fetchable, and the close lists it. To hand the close to the members, start the window again on the journal, or run the close with `--socket`: once a member's queue is empty, every `placket get` returns the close and keeps it as `close.json`.

## 2. Found the successor under a new key

Make the new key, and a policy that names the old Accord and its close in `supersedes`:

```sh
placket key new --out mediator2.key >mediator2.ref
jq -c --arg accord "$accord" --arg close "$close" '. + {supersedes: {accord: $accord, close: $close}}' policy.json >policy2.json
echo "{\"actor\":\"$(cat mediator2.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator2.json
```

A binding names its mediator and its policy, so each party's principal binds it again; the declarations, and their seals, are the same:

```sh
for a in proposer applier decider; do
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat mediator2.ref)" --until 2099-01-01T00:00:00Z \
		--policy policy2.json >"$a.binding2"
done
placket founding --policy policy2.json --mediator mediator2.json --card card.json \
	--seal proposer.seal --seal applier.seal --seal decider.seal \
	--binding proposer.binding2 --binding applier.binding2 --binding decider.binding2 >founding2.json
```

Every party should read `founding2.json` before it uses the new Accord: `placket verify --founding founding2.json` on its first journal names the earlier Accord and close it supersedes.

## 3. Check the old journal against the pinned close

```sh
placket verify --receipts journal --accord "$accord" --founding founding.json --superseded-by founding2.json
```

It exits 0 and prints `complete: against close receipt <close>, which successor accord <new id> pins; not claimed: that the mediator signed no other log for this Accord`. A journal whose close is not the one the successor pins, a successor that supersedes nothing or another Accord, and a journal with no close are tool error `verify superseded`.

## 4. Run the successor, and retire the old key

Start the new Accord's window with the new key, the same openings and a new journal. It starts the new Accord proposed: each party views and accepts its view of it, as for the first, and the last acceptance founds it and writes `acceptances.json` beside `founding2.json`, so keep the two Accords' files in directories of their own.

```
placket window --key mediator2.key --founding founding2.json --openings openings --socket /tmp/plk.s --journal journal2
```

Keep the old journal and its `founding.json`: they are the old Accord's record. Then destroy `mediator.key`:

```
rm mediator.key
```

Removing the file may not destroy the key. A backup, a copy made when the key was moved, or the file system itself (a journal, a snapshot, a block not yet written over) can keep its bytes, and whoever finds them holds the old key. The key is retired only once every copy is gone: remove each copy and backup too, and treat any you cannot remove as a key that may be stolen (below).

## What this does not protect

Whoever stole the old key before it was destroyed can still sign single receipts for the old Accord, which verify on their own, and a whole other journal with another close. Against a close a successor pins, only the journal that close pins is complete, so check a journal of the old Accord with `--superseded-by`, never a receipt alone (`docs/security/threat-model.md`, L19). Rotation within one Accord, and a close the members countersign, are later work.
