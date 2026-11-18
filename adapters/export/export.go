// Package export makes the log of a journal (log-1, log-2): one row per question, in the order the
// questions were asked, and with all a row for every crossing besides, in journal order, as JSON or
// CSV. Every cell comes from a receipt that verified under the mediator's key or
// from a kept part such a receipt covers (log-from-receipts): nothing from a file name, the clock or a
// line no receipt covers. The answer and the release are read from receipts alone, by
// receipts.Releases, so a state directory adds only words; a verdict is never decided again
// (one-decision-module).
//
// The columns are a door format, not a frozen one:
//
//	proposal       the question's id, the id of its ask's receipt
//	topic          the asker's topic, from the asker's kept message
//	kind           the asked part's kind
//	asked_at       the ask's time
//	answered_at    the closing's time; empty while open
//	answer         accepted, changed, declined, cancelled, timeout, undeliverable, blocked or open,
//	               from the closing receipt, addressed to the asker
//	released_at    the release receipt's time, when it passed
//	comment        the text of a comment the answerer gave the asker about the question, from the
//	               giver's kept message, since only an envelope covers its re
//	answer_signed  yes when an answer call whose typed hash the closing's by names reads, from the
//	               journal's answers.jsonl, a kept answer line or a kept delivery (receipt-11)
//	answerer       the closing receipt's from
//	receipts       the ids of every receipt the row rests on
//	text           with words: the asked part's text
//	changed_text   with words: the changed version's text, from any kept part its digest covers
//
// With all, every row has these columns after receipts, from the receipt of a crossing's row or of a
// question's ask, and a crossing's row leaves the question's columns empty:
//
//	at             the receipt's time
//	from, to       the receipt's from and to
//	word           pass or block, the receipt's verdict, or ask for a question's row
//	reason         the receipt's reason
//	re             the part's re, from the giver's kept message
//
// A kept part is used only when a verified receipt covers it: a part the actor gave by its
// envelope's hash (its body, kind, topic and re), a part it fetched or a version it gave as an
// answer by the salted digest (its body alone). A kept part that no receipt covers is left out, with
// a note naming the receipt. An answer call that does not read, or that no closing names, is not
// evidence of anything, and is passed over.
package export

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// The words of the answer column besides the core's own (product-words-once): an answer or a
// version that did not cross to the asker, a question still open, and a verified answer call.
const (
	AnswerBlocked = "blocked"
	AnswerOpen    = "open"
	Signed        = "yes"
	// WordAsk is the word column of a question's row, the product word of the verdict escalate
	// (log-2).
	WordAsk = "ask"
)

// Code says why no log was made.
type Code string

// The codes of Error.
const (
	// CodeVerify: the receipts do not verify; Core is the verifier's code and Index its receipt.
	CodeVerify Code = "verify"
	// CodeAccord: the receipts name more than one Accord; Index is the first of another.
	CodeAccord Code = "accord"
	// CodeCard: the receipts name more than one card; Index is the first of another.
	CodeCard Code = "card"
)

// Error is a journal the log refuses. It holds nothing a receipt or a part holds.
type Error struct {
	Code  Code
	Core  string
	Index int
}

func (e *Error) Error() string {
	s := "export: " + string(e.Code)
	if e.Core != "" {
		s += ": " + e.Core
	}
	return s + ", index " + strconv.Itoa(e.Index)
}

// A Row is one question's row, or with all one crossing's (log-1, log-2).
type Row struct {
	Proposal, Topic, Kind, AskedAt, AnsweredAt, Answer, ReleasedAt, Comment, AnswerSigned, Answerer string
	Receipts                                                                                        []string
	Text, ChangedText                                                                               string
	// With all: the receipt's time, parties, word and reason, and the part's re (log-2).
	At, From, To, Word, Reason, Re string
}

// A Log is the rows of one journal, its Accord and its mediator. All says it holds a row for every
// crossing, and that its rows print the columns of all (log-2).
type Log struct {
	Accord, Mediator string
	Rows             []Row
	All              bool
	// Close is the id of the close receipt the journal ends in, and empty when it has none; the
	// close is no row (close-2, docs/adr/0022-the-close.md).
	Close string
}

// Closing is the word for how a question closed, read from its closing receipt alone: accepted or
// changed for an answer that passed, by its kind; declined, cancelled, timeout, undeliverable or
// closed by the closing's reason (close-1); and blocked for an answer or a version that did not
// cross to the asker.
func Closing(p receipts.Predicate, s receipts.Subject) string {
	if p.Verdict == string(composition.Pass) {
		if s.Name == receipts.NameChanged {
			return string(answer.Changed)
		}
		return string(answer.Accepted)
	}
	switch p.Reason {
	case receipts.ReasonDeclined, receipts.ReasonCancelled, receipts.ReasonTimeout, receipts.ReasonUndeliverable,
		receipts.ReasonClosed:
		return p.Reason
	}
	return AnswerBlocked
}

// A part a state directory kept, read and covered.
type keptPart struct {
	receipt    string // the id of the receipt that covers it
	given      bool
	kind       string
	topic, re  string
	text       string
	salt       []byte
	answerCall []byte // the answer call inside a kept delivery, when an answer queued it
}

// Build makes the log of the receipts, verified under the mediator's reference, with the kept parts,
// each one line of a kept.jsonl, and the answer calls, each one line of a journal's answers.jsonl:
// one row per question (log-1). It verifies first and makes no row when the receipts fail, none when
// they name more than one Accord or card, and none when receipts.Releases refuses them. Each kept
// part left out is noted; a kept answer call, a line of its own, is no part and is never noted.
func Build(mediatorRef string, list, kept, answers [][]byte, note func(string)) (Log, error) {
	return build(mediatorRef, list, kept, answers, false, note)
}

// BuildAll makes the log as Build does, with a row for every crossing that belongs to no question
// besides the questions' rows, each where its receipt stands in the journal, and the columns of all
// (log-2).
func BuildAll(mediatorRef string, list, kept, answers [][]byte, note func(string)) (Log, error) {
	return build(mediatorRef, list, kept, answers, true, note)
}

func build(mediatorRef string, list, kept, answers [][]byte, all bool, note func(string)) (Log, error) {
	verified, err := receipts.Verify(mediatorRef, list)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return Log{}, &Error{Code: CodeVerify, Core: string(re.Code), Index: re.Index}
		}
		return Log{}, &Error{Code: CodeVerify, Index: -1}
	}
	st := make([]receipts.Statement, len(verified))
	accords, cards := make([]string, len(verified)), make([]string, len(verified))
	closeID := ""
	for i, v := range verified {
		st[i] = v.Statement()
		accords[i], cards[i] = st[i].Predicate.Accord, st[i].Predicate.Card
		// A close is no row of the log (RoleClose), but it names the Accord too (close-2).
		if c, ok := v.Close(); ok {
			accords[i], cards[i] = c.Accord, c.Card
			closeID = c.ID
		}
	}
	// One journal is one Accord's: receipts of two are refused whole.
	for i := range st {
		switch {
		case accords[i] != accords[0]:
			return Log{}, &Error{Code: CodeAccord, Index: i}
		case cards[i] != cards[0]:
			return Log{}, &Error{Code: CodeCard, Index: i}
		}
	}
	places, err := receipts.Releases(verified)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return Log{}, &Error{Code: CodeVerify, Core: string(re.Code), Index: re.Index}
		}
		return Log{}, &Error{Code: CodeVerify, Core: string(receipts.CodeRelease), Index: -1}
	}
	lg := Log{Mediator: mediatorRef, Rows: []Row{}, All: all, Close: closeID}
	if len(st) > 0 {
		lg.Accord = accords[0]
	}
	byID := map[string]int{}
	for i, s := range st {
		if s.ID != "" {
			byID[s.ID] = i
		}
	}
	var parts []keptPart
	// The answer calls that read, by their typed hash (receipt-11): the journal's, the ones a key
	// kept, and the ones inside the deliveries it kept.
	calls := map[string]answer.Call{}
	signed := func(b []byte) {
		if c, err := answer.Read(b); err == nil {
			calls[answer.ID(b)] = c
		}
	}
	for _, b := range answers {
		signed(b)
	}
	for _, line := range kept {
		if call, ok := keptAnswer(line); ok {
			signed(call)
			continue
		}
		p, id, ok := readKept(line, st, byID)
		if !ok {
			if id != "" {
				note("a kept part of receipt " + id + " is covered by no receipt; left out")
			} else {
				note("a kept line is not a kept part; left out")
			}
			continue
		}
		parts = append(parts, p)
		if p.answerCall != nil {
			signed(p.answerCall)
		}
	}

	for qi, pl := range places {
		if pl.Role == receipts.RoleGive && all {
			lg.Rows = append(lg.Rows, crossingRow(st[qi], parts))
			continue
		}
		if pl.Role != receipts.RoleQuestion {
			continue
		}
		q := st[qi]
		row := Row{Proposal: q.ID, Kind: q.Subject.Name, AskedAt: q.Predicate.At, Answer: AnswerOpen, Receipts: []string{q.ID}}
		if all {
			row.At, row.From, row.To, row.Word, row.Reason = q.Predicate.At, q.Predicate.From, q.Predicate.To, WordAsk, q.Predicate.Reason
		}
		var closing, release *receipts.Statement
		for j := qi + 1; j < len(places); j++ {
			if places[j].Question != qi {
				continue
			}
			switch places[j].Role {
			case receipts.RoleClosing:
				closing = &st[j]
			case receipts.RoleRelease:
				release = &st[j]
			}
		}
		if closing != nil {
			row.AnsweredAt, row.Answerer = closing.Predicate.At, closing.Predicate.From
			row.Answer = Closing(closing.Predicate, closing.Subject)
			row.Receipts = append(row.Receipts, closing.ID)
			if by := closing.Predicate.By; by.Decider == receipts.ByAnswer {
				if c, ok := calls[by.Answer]; ok && fits(c, *closing, q, lg.Accord) {
					row.AnswerSigned = Signed
				}
			}
		}
		if release != nil {
			row.Receipts = append(row.Receipts, release.ID)
			if release.Predicate.Verdict == string(composition.Pass) {
				row.ReleasedAt = release.Predicate.At
			}
		}
		for _, p := range parts {
			switch {
			case p.given && p.receipt == q.ID:
				row.Topic, row.Text = p.topic, p.text
				if all {
					row.Re = p.re
				}
			case p.given && p.kind == kindComment && p.re == q.ID && closing != nil &&
				st[byID[p.receipt]].Predicate.From == closing.Predicate.From &&
				st[byID[p.receipt]].Predicate.To == q.Predicate.From &&
				st[byID[p.receipt]].Predicate.Verdict == string(composition.Pass) && row.Comment == "":
				row.Comment = p.text
				row.Receipts = append(row.Receipts, p.receipt)
			case !p.given && release != nil && p.receipt == release.ID && q.Subject.Name == p.kind && row.Text == "":
				row.Text = p.text
			}
			if !p.given && closing != nil && row.Answer == string(answer.Changed) &&
				st[byID[p.receipt]].Subject.Digest == closing.Subject.Digest {
				row.ChangedText = p.text
			}
		}
		lg.Rows = append(lg.Rows, row)
	}
	return lg, nil
}

const kindComment = "comment"

// crossingRow is the row of a crossing that belongs to no question: its kind, time, parties, verdict
// and reason from its receipt, and its topic, re and text from a kept part the receipt covers, a part
// its giver kept before one its receiver fetched (log-2). readKept has joined each part to its
// receipt, by the envelope's hash or the salted digest; nothing here matches by time.
func crossingRow(s receipts.Statement, parts []keptPart) Row {
	p := s.Predicate
	row := Row{Kind: s.Subject.Name, Receipts: []string{s.ID}, At: p.At, From: p.From, To: p.To, Word: p.Verdict, Reason: p.Reason}
	given := false
	for _, k := range parts {
		switch {
		case k.receipt != s.ID:
		case k.given && !given:
			given = true
			row.Topic, row.Re, row.Text = k.topic, k.re, k.text
		case !k.given && !given && row.Text == "":
			row.Text = k.text
		}
	}
	return row
}

// keptAnswer reads a kept line of an answer call its key signed, {"answer","salt"}, and returns the
// call's bytes in canonical form. It reports false for any other line: a kept part has a receipt, and
// an answer line none. It checks no signature.
func keptAnswer(line []byte) ([]byte, bool) {
	var k struct {
		Answer json.RawMessage `json:"answer"`
		Salt   *string         `json:"salt"`
	}
	if _, err := canonical.Form(line); err != nil || !strict.Decode(line, &k) || len(k.Answer) == 0 || k.Answer[0] != '{' || k.Salt == nil {
		return nil, false
	}
	c, err := canonical.Form(k.Answer)
	if err != nil {
		return nil, false
	}
	return c, true
}

// fits reports whether an answer call that read is the one the closing of question q says decided
// it: of this Accord and this question, with the action the closing records. An accept or a change
// is signed by the closing's from, and its digest is the closing's subject digest; a cancel is
// signed by the asker. A decline may be signed by any answerer of the question, while its closing's
// from is always the first; the log reads no founding set to know the answerers, and the closing's
// by.answer already binds this call to it, so a decline's signer is not compared. A closing the
// mediator decided has no call.
func fits(c answer.Call, closing, q receipts.Statement, accord string) bool {
	if c.Accord != accord || c.Question != q.ID {
		return false
	}
	switch closing.Predicate.Reason {
	case receipts.ReasonDeclined:
		return c.Action == answer.Declined
	case receipts.ReasonCancelled:
		return c.Action == answer.Cancelled && c.From == q.Predicate.From
	case receipts.ReasonTimeout, receipts.ReasonUndeliverable:
		return false
	}
	want := answer.Accepted
	if closing.Subject.Name == receipts.NameChanged {
		want = answer.Changed
	}
	return c.Action == want && c.From == closing.Predicate.From && c.Digest == closing.Subject.Digest
}

// readKept reads one kept line and checks that a verified receipt covers it. It returns the id of
// the receipt it names, when it names one.
func readKept(line []byte, st []receipts.Statement, byID map[string]int) (keptPart, string, bool) {
	if _, err := canonical.Form(line); err != nil {
		return keptPart{}, "", false
	}
	receipt, ok := strict.Member(line, "receipt")
	if !ok {
		return keptPart{}, "", false
	}
	message, given := strict.Member(line, "message")
	rs, err := receipts.Decode(receipt)
	if err != nil {
		return keptPart{}, "", false
	}
	id := rs.ID
	i, ok := byID[id]
	if !ok {
		return keptPart{}, id, false
	}
	s := st[i]
	if given {
		p, ok := readGiven(line)
		var m64 string
		if !ok || json.Unmarshal(message, &m64) != nil {
			return keptPart{}, id, false
		}
		msg, _ := base64.StdEncoding.DecodeString(m64)
		c, err := canonical.Form(msg)
		if err != nil {
			return keptPart{}, id, false
		}
		h, _ := canonical.TypedHash(c)
		if h != s.Predicate.Envelope.Hash || p.kind != s.Subject.Name {
			return keptPart{}, id, false
		}
		p.receipt = id
		return p, id, true
	}
	p, body, ok := readFetched(line)
	if !ok || p.kind != s.Subject.Name || receipts.Digest(p.salt, body) != s.Subject.Digest {
		return keptPart{}, id, false
	}
	p.receipt = id
	return p, id, true
}

// readGiven reads a kept line of a part the actor gave, {"message","receipt"}: the message's kind,
// topic, re and text. It checks no receipt.
func readGiven(line []byte) (keptPart, bool) {
	var k struct {
		Message *string         `json:"message"`
		Receipt json.RawMessage `json:"receipt"`
	}
	if !strict.Decode(line, &k) || k.Message == nil || len(k.Receipt) == 0 {
		return keptPart{}, false
	}
	msg, err := base64.StdEncoding.Strict().DecodeString(*k.Message)
	if err != nil {
		return keptPart{}, false
	}
	var m struct {
		MessageID  json.RawMessage            `json:"messageId"`
		Role       json.RawMessage            `json:"role"`
		Extensions json.RawMessage            `json:"extensions"`
		Metadata   map[string]json.RawMessage `json:"metadata"`
		Parts      []json.RawMessage          `json:"parts"`
	}
	if !strict.Decode(msg, &m) || len(m.Parts) != 1 {
		return keptPart{}, false
	}
	x := m.Metadata[mediator.Extension]
	p, _, ok := readBody(m.Parts[0])
	if !ok {
		return keptPart{}, false
	}
	p.given, p.topic, p.re = true, stringMember(x, "topic"), stringMember(x, "re")
	var part struct {
		Text     json.RawMessage            `json:"text"`
		Raw      json.RawMessage            `json:"raw"`
		Data     json.RawMessage            `json:"data"`
		URL      json.RawMessage            `json:"url"`
		Metadata map[string]json.RawMessage `json:"metadata"`
	}
	if !strict.Decode(m.Parts[0], &part) {
		return keptPart{}, false
	}
	p.kind = stringMember(part.Metadata[mediator.Extension], "kind")
	return p, true
}

// stringMember is the string member of the object b named name, exactly, or "" when it has none.
func stringMember(b []byte, name string) string {
	m, ok := strict.Member(b, name)
	var s string
	if !ok || json.Unmarshal(m, &s) != nil {
		return ""
	}
	return s
}

// readBody reads a part's or a delivery's body: its text for a text body, a data body's canonical
// JSON, and nothing for a raw body; and the bytes the salted digest covers.
func readBody(b []byte) (keptPart, []byte, bool) {
	var o map[string]json.RawMessage
	if !strict.Decode(b, &o) {
		return keptPart{}, nil, false
	}
	var p keptPart
	n := 0
	var body []byte
	if v, has := o["text"]; has {
		n++
		if json.Unmarshal(v, &p.text) != nil {
			return keptPart{}, nil, false
		}
		body = []byte(p.text)
	}
	if v, has := o["raw"]; has {
		n++
		var s string
		if json.Unmarshal(v, &s) != nil {
			return keptPart{}, nil, false
		}
		r, err := base64.StdEncoding.Strict().DecodeString(s)
		if err != nil {
			return keptPart{}, nil, false
		}
		body = r
	}
	if v, has := o["data"]; has {
		n++
		c, err := canonical.Form(v)
		if err != nil {
			return keptPart{}, nil, false
		}
		p.text, body = string(c), c
	}
	return p, body, n == 1
}

// readFetched reads a kept line of a part the actor fetched or a version it gave as an answer, as
// the delivery holds it, the answer call inside it when an answer queued it: its kind, re, salt and
// text, the answer call, and the bytes the salted digest covers. It checks no receipt.
func readFetched(line []byte) (keptPart, []byte, bool) {
	var k struct {
		Answer    json.RawMessage `json:"answer"`
		MediaType *string         `json:"mediaType"`
		Kind      *string         `json:"kind"`
		MessageID *string         `json:"messageId"`
		Re        *string         `json:"re"`
		Receipt   json.RawMessage `json:"receipt"`
		ReceiptID *string         `json:"receiptId"`
		Salt      *string         `json:"salt"`
		Text      json.RawMessage `json:"text"`
		Raw       json.RawMessage `json:"raw"`
		Data      json.RawMessage `json:"data"`
	}
	if !strict.Decode(line, &k) || k.Kind == nil || k.Salt == nil || k.MessageID == nil || len(k.Receipt) == 0 {
		return keptPart{}, nil, false
	}
	salt, err := hex.DecodeString(*k.Salt)
	if err != nil || len(salt) != mediator.SaltBytes {
		return keptPart{}, nil, false
	}
	p, body, ok := readBody(line)
	if !ok {
		return keptPart{}, nil, false
	}
	p.kind, p.salt = *k.Kind, salt
	if k.Re != nil {
		p.re = *k.Re
	}
	if len(k.Answer) > 0 {
		if c, err := canonical.Form(k.Answer); err == nil {
			p.answerCall = c
		}
	}
	return p, body, true
}

// The columns, in order: a question's, then all's only with all, and the last two only with words.
var (
	columns    = []string{"proposal", "topic", "kind", "asked_at", "answered_at", "answer", "released_at", "comment", "answer_signed", "answerer", "receipts"}
	allColumns = []string{"at", "from", "to", "word", "reason", "re"}
	words      = []string{"text", "changed_text"}
)

// header is the log's columns, in order.
func (l Log) header(withWords bool) []string {
	h := append([]string(nil), columns...)
	if l.All {
		h = append(h, allColumns...)
	}
	if withWords {
		h = append(h, words...)
	}
	return h
}

func (r Row) cells(all, withWords, noWords bool) []string {
	comment := r.Comment
	if noWords {
		comment = ""
	}
	c := []string{r.Proposal, r.Topic, r.Kind, r.AskedAt, r.AnsweredAt, r.Answer, r.ReleasedAt, comment, r.AnswerSigned, r.Answerer, strings.Join(r.Receipts, " ")}
	if all {
		c = append(c, r.At, r.From, r.To, r.Word, r.Reason, r.Re)
	}
	if withWords {
		c = append(c, r.Text, r.ChangedText)
	}
	return c
}

// JSON writes the log as one object, {"accord","claim","mediator","notClaimed","rows"}, each row
// an object of the columns, receipts a list; claim and notClaimed are the lines the caller gives.
func (l Log) JSON(claim, notClaimed string, withWords, noWords bool) []byte {
	rows := []map[string]any{}
	names := l.header(withWords)
	for _, r := range l.Rows {
		o := map[string]any{}
		for i, c := range r.cells(l.All, withWords, noWords) {
			o[names[i]] = c
		}
		o["receipts"] = append([]string{}, r.Receipts...)
		rows = append(rows, o)
	}
	o := map[string]any{"accord": l.Accord, "mediator": l.Mediator, "claim": claim, "notClaimed": notClaimed, "rows": rows}
	if l.Close != "" {
		o["close"] = l.Close
	}
	b, _ := json.Marshal(o)
	c, _ := canonical.Form(b)
	return c
}

// formulaStarts are the characters a cell may not begin with, lest a spreadsheet read it as a
// formula: the four that start one, and the tab and the carriage return, which a spreadsheet may
// drop before it looks.
const formulaStarts = "=+-@\t\r"

// CSV writes the log as a header and one record per row, quoted per RFC 4180. A cell that begins
// with =, +, -, @, a tab or a carriage return is written after a ', so that no spreadsheet runs it
// as a formula.
func (l Log) CSV(w io.Writer, withWords, noWords bool) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(l.header(withWords)); err != nil {
		return err
	}
	for _, r := range l.Rows {
		cells := r.cells(l.All, withWords, noWords)
		for i, c := range cells {
			if c != "" && strings.ContainsRune(formulaStarts, rune(c[0])) {
				cells[i] = "'" + c
			}
		}
		if err := cw.Write(cells); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
