// Package local is the window's door: a unix socket on which members of one Accord call its
// mediator. One connection carries one call: the client connects, writes one call and a line feed,
// reads one reply and a line feed, and closes. The call and the reply are door formats, not frozen
// ones, but for the answer call (below); decision record 0010 says why.
//
// A call, at most MaxCall bytes, is {"call","sig"}:
//
//	call.op      give, get, questions, resolve or result
//	call.caller  the actor reference of the key that signs the call
//	call.accord  the Accord id the caller expects
//	call.at      the caller's clock, a timestamp in whole seconds
//	call.nonce   16 random bytes in lowercase hex
//	call.args    by op. give: {"envelope","message"}, each standard base64 of the exact bytes the
//	             caller signed; resolve: {"action","answer"?,"question"}, the action accepted,
//	             changed, declined or cancelled, and with accepted or changed the answer's text;
//	             result: {"question"}; get and questions: {}
//	sig          standard base64 of signing.SignPayload(PayloadType, the call's canonical bytes)
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
// A reply is one of:
//
//	{"ok":true,"outcome":O,"release"?:O,"salt"?:S}  give and resolve, and result for a closed
//	                                                question, to its asker: O is {"reason",
//	                                                "receipt","receiptId","verdict"}, the receipt as
//	                                                the mediator wrote it; release is the release
//	                                                receipt's; salt, in lowercase hex, the 32-byte
//	                                                salt the door minted for an accept or a change
//	{"ok":true,"delivery":D,"answerCall"?:A}        get, with a delivery queued: D the delivery as
//	                                                the mediator returned it, and beside it A, the
//	                                                answer call, when D's body is that call's answer
//	{"ok":true,"questions":[Q...]}                  questions: Q is {"asker","deadline","form","id",
//	                                                "kind","message"?,"size"?}; a text part's
//	                                                message is its text, a data part's its canonical
//	                                                JSON, and a raw part is shown by its size alone;
//	                                                the reply fits MaxReply, so past it a text or
//	                                                data part too is shown by its size, and the
//	                                                newest questions may be left out
//	{"ok":true}                                     get with nothing queued; result otherwise
//	{"ok":false,"rejection":C}                      the mediator's rejection
//	{"ok":false,"error":C}                          the mediator's error: its caller's misuse
//	{"ok":false,"door":C}                           a call the door refused
//
// The answer call is a resolve call that accepts or changes. The door attaches it to each delivery
// whose body is its answer, and nowhere else (answer-call-attached), as {"call","sig"}: the call's
// canonical bytes in standard base64 and its sig as the door received it. A party that keeps the
// delivery keeps the answerer's own signature over the question, the action and the text, so the
// answer call is a frozen format: its signed members are those above, under PayloadType.
package local

// PayloadType is the payload type a call is signed under (call-signing). It is a media type, not a
// placket type URI: signing.SignPayload refuses those, so no call's signature can stand for a
// placket object's.
const PayloadType = "application/vnd.placket.call+json"
