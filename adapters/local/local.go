// Package local is the window's door: a unix socket on which members of one Accord call its
// mediator. One connection carries one call: the client connects, writes one call and a line feed,
// reads one reply and a line feed, and closes. The call and the reply are door formats, not frozen
// ones, but for the answer call (below); decision record 0010 says why.
//
// A call, at most MaxCall bytes, is {"call","sig"}, and a reply one line of at most MaxReply bytes.
// Their members, by op, are the mediator API's wire layer: the schemas under api/wire/, which are
// the one source of the door formats, and docs/reference/mediator-api.md, generated from them
// (docs/adr/0024-the-mediator-api.md). A call of questions, get or result may wait at the door up to
// MaxWait seconds; every questions reply carries a cursor; and a get that waits replies queued, and
// never takes a delivery itself.
//
// The door checks a call in this order and stops at the first that fails (door-check-order):
// its size (too-large), its form (bad-call), its signature under call.caller (bad-signature), its
// Accord (wrong-accord), its time within StaleSeconds of the door's (stale), and its nonce, unseen
// in the last ReplaySeconds (replayed). Then a give from a caller whose last give is still with a
// judge is refused (judge-busy), so a caller's gives reach the mediator in the order of their seq.
// Only then does the mediator see it, and the mediator's first row is membership. A refused call
// changes nothing, but that a give refused judge-busy has spent its nonce. The door hands the mediator the operation's
// bytes exactly as decoded from the call's base64 (door-passes-bytes).
//
// A reply carries the call's outcomes, a delivery, the questions, the close, queued, nothing, or a
// refusal under rejection, error or door, as api/wire/<op>.reply.json says.
//
// The answer call a resolve carries is the answerer's own signed call, a frozen format
// (spec/frozen-formats.md): the window takes it as the call carried it, and it travels inside every
// delivery its answer queues, as the delivery's answer (answer-call-attached, decision record 0021).
package local

// PayloadType is the payload type a call is signed under (call-signing). It is a media type, not a
// placket type URI: signing.SignPayload refuses those, so no call's signature can stand for a
// placket object's.
const PayloadType = "application/vnd.placket.call+json"
