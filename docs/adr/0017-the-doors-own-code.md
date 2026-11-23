# 0017. The door's own failures have a code of their own, and a reply has a size limit

Status: accepted. Date: 2026-11-17. Amends record 0010. Amended by record 0021.

## Context

Record 0010 gives the door its own codes, carried under the reply's `door` member, apart from the mediator's rejections and errors (`rejection-vs-error`). It names the codes for a call the door refuses. It does not say what the door replies when it fails itself: its salt source fails, a reply cannot be encoded, or an error comes back from the mediator that is neither a `*mediator.Rejection` nor a `*mediator.Error`. In those cases the door replied as the mediator's `internal`, which tells the caller the core failed when it did not, and mixes the door's failure into the mediator's codes.

Record 0010 also bounds a call at 1 MiB, but sets no bound on a reply. The client read a reply with no limit, so a window, or anything listening at its path, could make the client read without end. Most replies are bounded by the call that made them, but the questions reply is not: it lists every open question the caller may answer, each with its part, and nothing bounds how many questions are open.

## Decision

- **`local.Internal`**, `const Internal DoorCode = "door-internal"`. Every failure of the door itself replies under the `door` member with this code. It is never sent as the mediator's `internal`. When even the reply cannot be encoded, the door writes the fixed reply `{"door":"door-internal","ok":false}`.
- **`local.MaxReply`**, `const MaxReply = 4 * MaxCall`. The client reads a reply through a bounded reader and refuses a longer one with its `reply` error. The bound holds one delivery, whose body came in under `MaxCall`, with its receipt and its answer call, with room to spare.
- **The questions reply fits `MaxReply`.** The window counts every open question first by its size alone, and leaves out any question, with every one after it, that would take the reply past `MaxReply` even so. Then, oldest first, a text or data question shows its message while the reply still fits, and is otherwise shown by its size in bytes, as a raw part always is. A question left out, or shown by its size, is shown in full once older questions close.
- This amends record 0010's decision on the door's codes and on the socket. The rest of record 0010 stands.

## Consequences

- A caller can tell the door's own failure from the core's, and a log of replies records them apart.
- The window sends no reply larger than `MaxReply`, so a client refuses only a reply no window sends.
- An answerer with many large open questions sees some of them by their size alone, or not yet at all, until older ones close.
- `door-internal` and `MaxReply` are door formats, which are not frozen (`format-change-noted`); they take a `CHANGELOG.md` line.
