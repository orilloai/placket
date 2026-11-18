package local_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// TestTheQuestionsReplyShowsWhatIsAsked asks about a text, a raw and a data part: the answerer's
// questions reply carries, for each, the ask's receipt, whose id is the question's, the part's salt
// and its body, raw included, whose salted digest is the receipt's subject digest; a member who is
// not an answerer gets none (maybe-1, U11).
func TestTheQuestionsReplyShowsWhatIsAsked(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	asks := []struct {
		id, form string
		value    any
		body     []byte
	}{
		{"p-1", "text", "Rename total.", []byte("Rename total.")},
		{"p-2", "raw", base64.StdEncoding.EncodeToString([]byte{0, 0xff, 7}), []byte{0, 0xff, 7}},
		{"p-3", "data", map[string]any{"b": 1, "a": "x"}, []byte(`{"a":"x","b":1}`)},
	}
	for i, a := range asks {
		msg := w.message(t, a.id, key(t, "applier").Reference(), "proposal", a.form, a.value, "")
		rep := w.call(t, "proposer", local.OpGive, local.GiveArgs(msg, envelopeOf(t, w, int64(i+1), msg)))
		if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
			t.Fatalf("%s was not asked: %+v", a.id, rep)
		}
	}
	rep := w.call(t, "decider", local.OpQuestions, nil)
	if len(rep.Questions) != len(asks) || rep.More {
		t.Fatalf("got %d questions, more %v", len(rep.Questions), rep.More)
	}
	for i, q := range rep.Questions {
		st, err := receipts.Decode(q.Receipt)
		body := q.Raw
		if q.Message != nil {
			body = []byte(*q.Message)
		}
		if err != nil || st.ID != q.ID || q.Form != asks[i].form || !bytes.Equal(body, asks[i].body) ||
			receipts.Digest(q.Salt, body) != st.Subject.Digest || *q.Size != len(asks[i].body) {
			t.Fatalf("question %d (%s): the body and salt do not make the receipt's digest", i, q.Form)
		}
	}
	if rep := w.call(t, "applier", local.OpQuestions, nil); !rep.IsQuestions || len(rep.Questions) != 0 {
		t.Fatalf("a member who is not an answerer is shown %d questions", len(rep.Questions))
	}
}

func envelopeOf(t *testing.T, w *world, seq int64, msg []byte) []byte {
	t.Helper()
	h, err := canonical.TypedHash(msg)
	if err != nil {
		t.Fatal(err)
	}
	w.seqs["proposer"] = seq
	return envelope(t, seq, h, key(t, "proposer"))
}

// TestTheJournalKeepsEveryAcceptedCall accepts with release and declines: the journal's
// answers.jsonl holds both calls, canonical, in the order of their closings, and no line of it holds
// a salt or a text; both deliveries the accept queued carry the call inside them, and no reply
// carries one beside a delivery. After a restart, with a call line no receipt names added, the
// window reloads and carries on (maybe-12, U8, U9, U31).
func TestTheJournalKeepsEveryAcceptedCall(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	win := start(t, w.window(t))
	q1 := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.").Outcome.ReceiptID
	q2 := w.give(t, "proposer", "applier", "p-2", "proposal", "Drop notes.").Outcome.ReceiptID
	accept := w.answerArgs(t, "decider", q1, answer.Accepted, "the planted answer")
	decline := w.answerArgs(t, "decider", q2, answer.Declined, "")
	for _, args := range []map[string]any{accept, decline} {
		if rep := w.call(t, "decider", local.OpResolve, args); rep.Outcome == nil {
			t.Fatalf("the answer was refused: %+v", rep)
		}
	}
	b, err := os.ReadFile(filepath.Join(w.journal, journal.FileAnswers))
	if err != nil {
		t.Fatal(err)
	}
	want := string([]byte(accept["answer"].(json.RawMessage))) + "\n" + string([]byte(decline["answer"].(json.RawMessage))) + "\n"
	if string(b) != want || strings.Contains(string(b), accept["salt"].(string)) || strings.Contains(string(b), "the planted answer") {
		t.Fatalf("answers.jsonl holds %q", b)
	}
	for _, n := range []string{"applier", "proposer"} {
		rep := w.call(t, n, local.OpGet, nil)
		if !bytes.Contains(rep.Delivery, []byte(`"answer":`+string([]byte(accept["answer"].(json.RawMessage))))) || rep.AnswerCall != nil {
			t.Fatalf("%s's delivery does not carry the call inside it: %s", n, rep.Delivery)
		}
	}
	win.stop(t)
	stray, _ := answer.Sign(answer.Call{Accord: w.accord, Question: q1, Action: answer.Declined}, key(t, "decider"))
	f, err := os.OpenFile(filepath.Join(w.journal, journal.FileAnswers), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(append(stray, '\n'))
	_ = f.Close()
	start(t, w.window(t))
	if rep := w.give(t, "proposer", "applier", "n-1", "note", "A note."); rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("after the restart: %+v", rep)
	}
}
