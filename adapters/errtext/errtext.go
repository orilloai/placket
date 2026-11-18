// Package errtext holds one sentence of cause and fix for every code placket mcp can return, in one
// table, so the MCP tools' error text and the mediator API's reference say the same. A row is keyed
// by where its code comes from: a code in a door reply by its member, door, rejection or error; every
// other code by the package whose Code type declares it. The same spelling under two keys has two
// rows, each saying what it means there. The command line keeps its own error text.
//
// It imports nothing of placket, so the reference's generator reads it without the adapters.
package errtext

import "sort"

// The keys of the codes in a door reply, by the reply's member.
const (
	FromDoor      = "door"
	FromRejection = "rejection"
	FromError     = "error"
)

// The keys of the other codes, by the package that declares them.
const (
	FromClient  = "adapters/client"
	FromMCP     = "adapters/mcp"
	FromState   = "adapters/state"
	FromKeyfile = "adapters/keyfile"
	FromLocal   = "adapters/local"
)

// A Row is one code's sentence: where the code comes from, the code, and the sentence.
type Row struct {
	From, Code, Sentence string
}

type key struct{ from, code string }

var table = map[key]string{
	// The door's own codes (adapters/local.DoorCode).
	{FromDoor, "too-large"}:     "The call is longer than the door reads; send a smaller part.",
	{FromDoor, "bad-call"}:      "The call is not in the door's format, or its wait is not a whole number from 0 to 60; check the call against the mediator API.",
	{FromDoor, "bad-signature"}: "The call's signature does not verify under its caller's key; sign it with the key it names.",
	{FromDoor, "wrong-accord"}:  "The window serves another Accord than the call names; check the mediator's Accord id.",
	{FromDoor, "stale"}:         "The call's time is more than 60 seconds from the window's clock; set the clock right and call again.",
	{FromDoor, "replayed"}:      "The window has seen the call's nonce already; call again with a fresh nonce.",
	{FromDoor, "door-internal"}: "The window failed after the call passed its checks; call again, and see the window's log if it fails again.",
	{FromDoor, "judge-busy"}:    "The caller's last give is still with a judge; wait for its reply, then give again.",

	// The mediator's rejections (core/mediator.RejectionCode).
	{FromRejection, "not-a-member"}:       "The key is not a member of the Accord; use a key the founding set binds.",
	{FromRejection, "envelope-signature"}: "The envelope's signature does not verify under the giver's key; sign it with the giving key.",
	{FromRejection, "not-canonical"}:      "The message is not in canonical form; send its canonical bytes.",
	{FromRejection, "envelope-hash"}:      "The envelope names another message than the one sent; sign the envelope over this message's hash.",
	{FromRejection, "envelope-sequence"}:  "The envelope's seq is not above the last the mediator took from this key; give at the next seq.",
	{FromRejection, "extension-missing"}:  "The message lacks placket's extension or its role; build it as a member gives one.",
	{FromRejection, "receiver-unknown"}:   "The receiver is not a member of the Accord; give to a member's actor reference.",
	{FromRejection, "accord-mismatch"}:    "The message names another Accord than the window's; name this mediator's Accord.",
	{FromRejection, "one-part"}:           "The message must carry exactly one part; send each part on its own.",
	{FromRejection, "one-body"}:           "The part must carry exactly one body, a text, a raw body or data; send one.",
	{FromRejection, "url-unsupported"}:    "A part given by URL does not cross; send its body.",
	{FromRejection, "part-metadata"}:      "The part's kind or salt is missing or malformed; give a kind and a salt of 32 bytes.",
	{FromRejection, "answer-call"}:        "The answer call does not read, names another Accord, or is not the caller's; sign it with the answering key for this Accord.",
	{FromRejection, "no-open-question"}:   "No question of that id is open; list the open questions, or ask for the result.",
	{FromRejection, "not-an-answerer"}:    "The key may not answer or cancel that question; only its answerers answer it, and only its asker cancels it, while it has answerers.",
	{FromRejection, "answer-body"}:        "The answer's text and salt do not fit its call; send the text whose salted digest the call signs.",
	{FromRejection, "closed"}:             "The Accord is closed and takes no give or answer; found its successor and use that.",

	// The mediator's errors (core/mediator.ErrorCode).
	{FromError, "at"}:        "A time does not keep the timestamp rule; the window's clock is wrong, so check it.",
	{FromError, "internal"}:  "The mediator could not write a receipt or delivery; see the window's log and restart it.",
	{FromError, "unfounded"}: "The window's mediator is not founded; restart the window on its founding set.",
	{FromError, "journal"}:   "A reloaded receipt does not verify or names another Accord; restore the journal or close the Accord.",
	{FromError, "held"}:      "A reloaded delivery is queued by no receipt; restore the journal's held files.",
	{FromError, "question"}:  "A reloaded open question does not match the receipts; restore the journal's questions file.",
	{FromError, "closed"}:    "The Accord is closed already; nothing more can close it.",
	{FromError, "close"}:     "The journal was cut inside a close; run placket window close to finish it.",
	{FromError, "salt"}:      "No mediator returns this code any more; update the window.",
	{FromError, "answer"}:    "No mediator returns this code any more; update the window.",
	{FromError, "action"}:    "No mediator returns this code any more; update the window.",

	// adapters/client.Code.
	{FromClient, "bad-config"}:         "The labels config is not in its format; fix mediators.json against api/config/mediators.json.",
	{FromClient, "label-taken"}:        "Two entries of the labels config have one label; give each entry its own label.",
	{FromClient, "mediator-collision"}: "Two entries of the labels config use one socket for two Accords, one state directory, or one Accord id with no label; give each its own socket, key or label.",
	{FromClient, "unknown-mediator"}:   "No mediator served has that name; name one by its label, or by its Accord id when it has none, as mediators lists them.",
	{FromClient, "reply"}:              "The window's reply is not what the call asked for; check the socket reaches this Accord's window.",
	{FromClient, "asked"}:              "The window does not show the question as its ask's receipt names it, so nothing was signed; list the questions again.",
	{FromClient, "rand"}:               "The source of randomness failed; call again.",
	{FromClient, "envelope"}:           "The envelope could not be signed; check the key file holds a key.",

	// adapters/mcp.Code.
	{FromMCP, "answering-key"}: "The label's key answers questions under its Accord, and placket mcp holds proposing keys only; serve a proposing key, or start it with --allow-answering.",
	{FromMCP, "founding"}:      "The label's founding set cannot be read or does not found; fix the founding path in the labels config.",
	{FromMCP, "bad-arguments"}: "The tool's arguments do not fit its input schema, or wait is above 60; send them as the schema says.",
	{FromMCP, "internal"}:      "The call failed in a way no other code names; call again, and report it if it persists.",

	// adapters/state.Code.
	{FromState, "state"}:       "A file of the state directory could not be read or written, or is not in its format; check the directory and its files.",
	{FromState, "state-actor"}: "The state directory belongs to another key; give each key its own state directory.",

	// adapters/keyfile.Code.
	{FromKeyfile, "key"}:        "The key file does not hold a key; give the path of a key placket key new made.",
	{FromKeyfile, "key-read"}:   "The key file could not be read; check its path and its owner.",
	{FromKeyfile, "key-exists"}: "A file is already at the key's path; choose another path.",
	{FromKeyfile, "key-write"}:  "The key file could not be written; check the directory.",
	{FromKeyfile, "key-mode"}:   "The key file can be read by others; make it mode 0600.",

	// adapters/local.Code.
	{FromLocal, "socket"}:      "The window could not be reached, or went away before it replied; start the window, or check the socket path.",
	{FromLocal, "reply"}:       "What the window sent is not a reply; check the socket is a placket window's.",
	{FromLocal, "socket-path"}: "The socket path is longer than the system takes; use a shorter one.",
	{FromLocal, "judge"}:       "The window's judge configs are not the ones its card lists; start it with the card's judges.",
	{FromLocal, "cursor"}:      "The window could not draw its cursors' random value; start it again.",
}

// Sentence is the sentence of the code from where it comes from, and whether it has one.
func Sentence(from, code string) (string, bool) {
	s, ok := table[key{from, code}]
	return s, ok
}

// Text is a code's error text as placket mcp returns it: the code, a colon and its sentence; the code
// alone when it has none.
func Text(from, code string) string {
	if s, ok := Sentence(from, code); ok {
		return code + ": " + s
	}
	return code
}

// Rows is every row, by where its code comes from, then by code.
func Rows() []Row {
	rows := make([]Row, 0, len(table))
	for k, s := range table {
		rows = append(rows, Row{From: k.from, Code: k.code, Sentence: s})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].From != rows[j].From {
			return rows[i].From < rows[j].From
		}
		return rows[i].Code < rows[j].Code
	})
	return rows
}
