// Package export makes the log of a journal: one row per question, in the order the questions were
// asked, as JSON or CSV. Every cell comes from a receipt that verified under the mediator's key or
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
//	answer_signed  yes when a kept answer call for the row verified
//	answerer       the closing receipt's from
//	receipts       the ids of every receipt the row rests on
//	text           with words: the asked part's text
//	changed_text   with words: the changed version's text, from any kept part its digest covers
//
// A kept part is used only when a verified receipt covers it: a part the actor gave by its
// envelope's hash (its body, kind, topic and re), a part it fetched or a version it gave as an
// answer by the salted digest (its body alone). A kept part that no receipt covers, and an answer
// call that does not verify, are left out, each with a note naming the receipt.
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
	"github.com/orilloai/placket/adapters/local"
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

// A Row is one question's row.
type Row struct {
	Proposal, Topic, Kind, AskedAt, AnsweredAt, Answer, ReleasedAt, Comment, AnswerSigned, Answerer string
	Receipts                                                                                        []string
	Text, ChangedText                                                                               string
}

// A Log is the rows of one journal, its Accord and its mediator.
type Log struct {
	Accord, Mediator string
	Rows             []Row
}

// Closing is the word for how a question closed, read from its closing receipt alone: accepted or
// changed for an answer that passed, by its kind; declined, cancelled, timeout or undeliverable by
// the closing's reason; and blocked for an answer or a version that did not cross to the asker.
func Closing(p receipts.Predicate, s receipts.Subject) string {
	if p.Verdict == string(composition.Pass) {
		if s.Name == receipts.NameChanged {
			return string(mediator.Changed)
		}
		return string(mediator.Accepted)
	}
	switch p.Reason {
	case receipts.ReasonDeclined, receipts.ReasonCancelled, receipts.ReasonTimeout, receipts.ReasonUndeliverable:
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
	answerCall []byte
}

// Build makes the log of the receipts, verified under the mediator's reference, with the kept parts,
// each one line of a kept.jsonl. It verifies first and makes no row when the receipts fail, none when
// they name more than one Accord or card, and none when receipts.Releases refuses them. Each kept part left out is noted.
func Build(mediatorRef string, list [][]byte, kept [][]byte, note func(string)) (Log, error) {
	st, err := receipts.Verify(mediatorRef, list)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return Log{}, &Error{Code: CodeVerify, Core: string(re.Code), Index: re.Index}
		}
		return Log{}, &Error{Code: CodeVerify, Index: -1}
	}
	// One journal is one Accord's: receipts of two are refused whole.
	for i, s := range st {
		switch {
		case s.Predicate.Accord != st[0].Predicate.Accord:
			return Log{}, &Error{Code: CodeAccord, Index: i}
		case s.Predicate.Card != st[0].Predicate.Card:
			return Log{}, &Error{Code: CodeCard, Index: i}
		}
	}
	places, err := receipts.Releases(st)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return Log{}, &Error{Code: CodeVerify, Core: string(re.Code), Index: re.Index}
		}
		return Log{}, &Error{Code: CodeVerify, Core: string(receipts.CodeRelease), Index: -1}
	}
	lg := Log{Mediator: mediatorRef, Rows: []Row{}}
	if len(st) > 0 {
		lg.Accord = st[0].Predicate.Accord
	}
	byID := map[string]int{}
	for i, s := range st {
		byID[s.ID] = i
	}
	var parts []keptPart
	for _, line := range kept {
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
	}

	for qi, pl := range places {
		if pl.Role != receipts.RoleQuestion {
			continue
		}
		q := st[qi]
		row := Row{Proposal: q.ID, Kind: q.Subject.Name, AskedAt: q.Predicate.At, Answer: AnswerOpen, Receipts: []string{q.ID}}
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
			case p.given && p.kind == kindComment && p.re == q.ID && closing != nil &&
				st[byID[p.receipt]].Predicate.From == closing.Predicate.From &&
				st[byID[p.receipt]].Predicate.To == q.Predicate.From &&
				st[byID[p.receipt]].Predicate.Verdict == string(composition.Pass) && row.Comment == "":
				row.Comment = p.text
				row.Receipts = append(row.Receipts, p.receipt)
			case !p.given && release != nil && p.receipt == release.ID && q.Subject.Name == p.kind && row.Text == "":
				row.Text = p.text
			}
			if !p.given && closing != nil && row.Answer == string(mediator.Changed) &&
				st[byID[p.receipt]].Subject.Digest == closing.Subject.Digest {
				row.ChangedText = p.text
			}
			if p.answerCall != nil && closing != nil && (row.Answer == string(mediator.Accepted) || row.Answer == string(mediator.Changed)) &&
				(p.receipt == closing.ID || (release != nil && p.receipt == release.ID)) {
				if signedBy(p, closing, lg.Accord, q.ID) {
					row.AnswerSigned = Signed
				} else {
					note("the answer call kept with receipt " + p.receipt + " does not verify; left out")
				}
			}
		}
		lg.Rows = append(lg.Rows, row)
	}
	return lg, nil
}

const kindComment = "comment"

// signedBy checks a kept answer call against its row: its signature under the row's answerer, its
// Accord, its question, its action against the answer's kind, and its answer, under the kept salt,
// against the closing receipt's digest.
func signedBy(p keptPart, closing *receipts.Statement, accord, question string) bool {
	sa, ok := local.VerifyAnswerCall(p.answerCall, closing.Predicate.From)
	if !ok || sa.Accord != accord || sa.Question != question {
		return false
	}
	want := string(mediator.Accepted)
	if closing.Subject.Name == receipts.NameChanged {
		want = string(mediator.Changed)
	}
	return sa.Action == want && receipts.Digest(p.salt, []byte(sa.Answer)) == closing.Subject.Digest
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
		Metadata map[string]struct {
			Topic string `json:"topic"`
			Re    string `json:"re"`
		} `json:"metadata"`
		Parts []json.RawMessage `json:"parts"`
	}
	if json.Unmarshal(msg, &m) != nil || len(m.Parts) != 1 {
		return keptPart{}, false
	}
	x := m.Metadata[mediator.Extension]
	p, _, ok := readBody(m.Parts[0])
	if !ok {
		return keptPart{}, false
	}
	p.given, p.topic, p.re = true, x.Topic, x.Re
	var meta struct {
		Metadata map[string]struct {
			Kind string `json:"kind"`
		} `json:"metadata"`
	}
	if json.Unmarshal(m.Parts[0], &meta) != nil {
		return keptPart{}, false
	}
	p.kind = meta.Metadata[mediator.Extension].Kind
	return p, true
}

// readBody reads a part's or a delivery's body: its text for a text body, a data body's canonical
// JSON, and nothing for a raw body; and the bytes the salted digest covers.
func readBody(b []byte) (keptPart, []byte, bool) {
	var o map[string]json.RawMessage
	if json.Unmarshal(b, &o) != nil {
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
// the delivery holds it, with answerCall beside it: its kind, re, salt and text, the answer call, and
// the bytes the salted digest covers. It checks no receipt.
func readFetched(line []byte) (keptPart, []byte, bool) {
	var k struct {
		AnswerCall json.RawMessage `json:"answerCall"`
		Kind       *string         `json:"kind"`
		MessageID  *string         `json:"messageId"`
		Re         *string         `json:"re"`
		Receipt    json.RawMessage `json:"receipt"`
		ReceiptID  *string         `json:"receiptId"`
		Salt       *string         `json:"salt"`
		Text       json.RawMessage `json:"text"`
		Raw        json.RawMessage `json:"raw"`
		Data       json.RawMessage `json:"data"`
	}
	if !strict.Decode(line, &k) || k.Kind == nil || k.Salt == nil || k.MessageID == nil || len(k.Receipt) == 0 {
		return keptPart{}, nil, false
	}
	salt, err := hex.DecodeString(*k.Salt)
	if err != nil || len(salt) < mediator.MinSaltBytes {
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
	if len(k.AnswerCall) > 0 {
		p.answerCall = append([]byte(nil), k.AnswerCall...)
	}
	return p, body, true
}

// The columns, in order; the last two only with words.
var (
	columns = []string{"proposal", "topic", "kind", "asked_at", "answered_at", "answer", "released_at", "comment", "answer_signed", "answerer", "receipts"}
	words   = []string{"text", "changed_text"}
)

func (r Row) cells(withWords, noWords bool) []string {
	comment := r.Comment
	if noWords {
		comment = ""
	}
	c := []string{r.Proposal, r.Topic, r.Kind, r.AskedAt, r.AnsweredAt, r.Answer, r.ReleasedAt, comment, r.AnswerSigned, r.Answerer, strings.Join(r.Receipts, " ")}
	if withWords {
		c = append(c, r.Text, r.ChangedText)
	}
	return c
}

// JSON writes the log as one object, {"accord","claim","mediator","notClaimed","rows"}, each row
// an object of the columns, receipts a list; claim and notClaimed are the lines the caller gives.
func (l Log) JSON(claim, notClaimed string, withWords, noWords bool) []byte {
	rows := []map[string]any{}
	names := columns
	if withWords {
		names = append(append([]string(nil), columns...), words...)
	}
	for _, r := range l.Rows {
		o := map[string]any{}
		for i, c := range r.cells(withWords, noWords) {
			o[names[i]] = c
		}
		o["receipts"] = append([]string{}, r.Receipts...)
		rows = append(rows, o)
	}
	b, _ := json.Marshal(map[string]any{"accord": l.Accord, "mediator": l.Mediator, "claim": claim, "notClaimed": notClaimed, "rows": rows})
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
	header := columns
	if withWords {
		header = append(append([]string(nil), columns...), words...)
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range l.Rows {
		cells := r.cells(withWords, noWords)
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
