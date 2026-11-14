package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
)

// allColumns are the columns --all adds to every row (log-2).
var allColumns = []string{"at", "from", "to", "word", "reason", "re"}

// sixMore is the worked example with three more crossings that belong to no question: a note the
// proposer gives the applier, which no term names, so it is blocked; and two comments the decider
// gives the proposer in a row, one kind, one sender, which pass and the proposer fetches.
type sixMore struct {
	*sixRun
	blocked, one, two string
}

func moreCrossings(t testing.TB) *sixMore {
	t.Helper()
	s := &sixMore{sixRun: six(t)}
	give := func(who, to, kind, text, topic string, code int) string {
		r := run(t, env(), s.as(who, "give", "--to", s.refs[to], "--kind", kind, "--text", text, "--topic", topic, "--keep", "--json")...)
		if r.code != code {
			t.Fatalf("give %s: %v", text, r)
		}
		var o struct{ Outcome struct{ ReceiptID string } }
		if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || o.Outcome.ReceiptID == "" {
			t.Fatalf("give %s: %v", text, r)
		}
		return o.Outcome.ReceiptID
	}
	s.blocked = give("proposer", "applier", "note", "Turn it off.", "ops", 1)
	s.one = give("decider", "proposer", "comment", "one", "t1", 0)
	s.two = give("decider", "proposer", "comment", "two", "t2", 0)
	s.drain(t, "proposer")
	return s
}

// A journalReceipt is what a test reads of one line of the journal: the receipt's id, the typed
// hash of its statement's canonical bytes, and its predicate and subject.
type journalReceipt struct {
	id, kind, verdict, reason, at, from, to, name string
	envelope                                      string
}

func readJournal(t testing.TB, dir string) []journalReceipt {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "receipts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []journalReceipt
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		var env struct{ Payload string }
		if err := json.Unmarshal([]byte(l), &env); err != nil {
			t.Fatal(err)
		}
		p, _ := base64.StdEncoding.DecodeString(env.Payload)
		c, err := canonical.Form(p)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := canonical.TypedHash(c)
		var st struct {
			Subject   []struct{ Name string }
			Predicate struct {
				Kind, Verdict, Reason, At, From, To string
				Envelope                            json.RawMessage
			}
		}
		if err := json.Unmarshal(c, &st); err != nil || len(st.Subject) != 1 {
			t.Fatalf("statement: %s", c)
		}
		pr := st.Predicate
		out = append(out, journalReceipt{id, pr.Kind, pr.Verdict, pr.Reason, pr.At, pr.From, pr.To, st.Subject[0].Name, string(pr.Envelope)})
	}
	return out
}

// rowsOf are the receipts that are rows of log --all, in journal order: every crossing that asks,
// and every crossing whose envelope is no question's; a closing and a release are not rows
// (receipt-6's rule, read here on its own).
func rowsOf(list []journalReceipt) []journalReceipt {
	asked := map[string]bool{}
	var rows []journalReceipt
	for _, r := range list {
		switch {
		case r.kind != "crossing", asked[r.envelope]:
		case r.verdict == "escalate":
			asked[r.envelope] = true
			rows = append(rows, r)
		default:
			rows = append(rows, r)
		}
	}
	return rows
}

// TestLogAllEveryCrossingInJournalOrder runs log --all on the worked example with three more
// crossings, in JSON and in CSV: a row for each ask, pass and block, in the order of the journal,
// none for a closing or a release; each row's at, from, to, word, reason and kind from its receipt;
// the question rows as without --all; and the block's row with the giver's topic and text from its
// kept message (log-2).
func TestLogAllEveryCrossingInJournalOrder(t *testing.T) {
	t.Parallel()
	s := moreCrossings(t)
	want := rowsOf(readJournal(t, s.journal()))
	for _, format := range []string{"json", "csv"} {
		rows, _ := logRows(t, s.sixRun, format, true, "--all", "--words")
		plain, _ := logRows(t, s.sixRun, format, true, "--words")
		if len(rows) != len(want) || len(want) != 4+2+3 {
			t.Fatalf("%s: %d rows, %d crossings", format, len(rows), len(want))
		}
		q := 0
		for i, row := range rows {
			w := want[i]
			word := map[string]string{"escalate": "ask", "pass": "pass", "block": "block"}[w.verdict]
			if strings.Fields(row["receipts"])[0] != w.id || row["at"] != w.at || row["from"] != w.from || row["to"] != w.to ||
				row["word"] != word || row["reason"] != w.reason || row["kind"] != w.name {
				t.Fatalf("%s row %d: %v, want %+v", format, i, row, w)
			}
			if word == "ask" {
				for k, v := range plain[q] {
					if row[k] != v {
						t.Fatalf("%s row %d: %s is %q, without --all %q", format, i, k, row[k], v)
					}
				}
				q++
				continue
			}
			for _, k := range []string{"proposal", "asked_at", "answered_at", "answer", "released_at", "comment", "answer_signed", "answerer", "changed_text"} {
				if row[k] != "" {
					t.Fatalf("%s row %d: %s is %q in a crossing's row", format, i, k, row[k])
				}
			}
			if row["receipts"] != w.id {
				t.Fatalf("%s row %d: receipts %q", format, i, row["receipts"])
			}
		}
		if q != 4 {
			t.Fatalf("%s: %d question rows", format, q)
		}
		byID := map[string]map[string]string{}
		for _, row := range rows {
			byID[strings.Fields(row["receipts"])[0]] = row
		}
		b := byID[s.blocked]
		if b["word"] != "block" || b["reason"] != "no-match" || b["topic"] != "ops" || b["text"] != "Turn it off." || b["re"] != "" ||
			b["from"] != s.refs["proposer"] || b["to"] != s.refs["applier"] || b["kind"] != "note" {
			t.Fatalf("%s: the block's row %v", format, b)
		}
		// The decider's comment on the changed proposal: a pass with re the question, a row of its
		// own beside its question's.
		var comment map[string]string
		for _, row := range rows {
			if row["kind"] == "comment" && row["re"] == s.changed {
				comment = row
			}
		}
		if comment == nil || comment["word"] != "pass" || comment["reason"] != "ok" || comment["text"] != "Someone reads it." ||
			comment["from"] != s.refs["decider"] || comment["to"] != s.refs["proposer"] {
			t.Fatalf("%s: the comment's row %v", format, comment)
		}
	}
}

// TestLogAllJoinsByDigestAndByEnvelope reads two comments of one kind from one sender, given in a
// row, with the receiver's kept record alone and with the giver's alone: each row's text is the
// text its receipt covers, by the salted digest for the part fetched and by the envelope's hash for
// the part given; a fetched part gives no topic (log-2).
func TestLogAllJoinsByDigestAndByEnvelope(t *testing.T) {
	t.Parallel()
	s := moreCrossings(t)
	row := func(format, who, id string) map[string]string {
		t.Helper()
		r := must(t, "log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--format", format, "--all", "--words", "--kept", s.state(who))
		var rows []map[string]string
		if format == "json" {
			var lg struct{ Rows []map[string]any }
			if err := json.Unmarshal([]byte(r.stdout), &lg); err != nil {
				t.Fatal(err)
			}
			for _, x := range lg.Rows {
				m := map[string]string{}
				for k, v := range x {
					if l, ok := v.([]any); ok {
						m[k] = l[0].(string)
					} else {
						m[k] = v.(string)
					}
				}
				rows = append(rows, m)
			}
		} else {
			recs, err := csv.NewReader(strings.NewReader(r.stdout)).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			for _, rec := range recs[1:] {
				m := map[string]string{}
				for i, c := range rec {
					m[recs[0][i]] = c
				}
				m["receipts"] = strings.Fields(m["receipts"])[0]
				rows = append(rows, m)
			}
		}
		for _, x := range rows {
			if x["receipts"] == id {
				return x
			}
		}
		t.Fatalf("%s: no row for %s", format, id)
		return nil
	}
	for _, format := range []string{"json", "csv"} {
		for id, want := range map[string][2]string{s.one: {"one", "t1"}, s.two: {"two", "t2"}} {
			if got := row(format, "proposer", id); got["text"] != want[0] || got["topic"] != "" || got["word"] != "pass" {
				t.Fatalf("%s: fetched %v, want text %q", format, got, want[0])
			}
			if got := row(format, "decider", id); got["text"] != want[0] || got["topic"] != want[1] {
				t.Fatalf("%s: given %v, want %q", format, got, want)
			}
			if got := row(format, "applier", id); got["text"] != "" || got["topic"] != "" {
				t.Fatalf("%s: no kept part, yet %v", format, got)
			}
		}
	}
}

// TestLogWithoutAllIsUnchanged runs log with and without --all on a journal of questions, passes
// and blocks, every way the log prints: without --all it prints, byte for byte, the question rows
// of --all with the columns of --all taken out, and no other row; standard error is the same
// (log-1).
func TestLogWithoutAllIsUnchanged(t *testing.T) {
	t.Parallel()
	s := moreCrossings(t)
	base := []string{"log", "--mediator", s.refs["mediator"], "--receipts", s.journal()}
	kept := []string{"--kept", s.state("proposer"), "--kept", s.state("applier"), "--kept", s.state("decider")}
	for _, more := range [][]string{nil, kept, append(kept[:6:6], "--words"), append(kept[:6:6], "--no-words")} {
		for _, format := range []string{"json", "csv"} {
			args := append(append(append([]string(nil), base...), more...), "--format", format)
			plain, all := must(t, args...), must(t, append(args, "--all")...)
			if plain.stderr != all.stderr {
				t.Fatalf("%v: stderr %q, with --all %q", more, plain.stderr, all.stderr)
			}
			var want string
			if format == "json" {
				var lg map[string]json.RawMessage
				if err := json.Unmarshal([]byte(all.stdout), &lg); err != nil {
					t.Fatal(err)
				}
				var rows, questions []map[string]json.RawMessage
				if err := json.Unmarshal(lg["rows"], &rows); err != nil {
					t.Fatal(err)
				}
				for _, r := range rows {
					if string(r["word"]) != `"ask"` {
						continue
					}
					for _, c := range allColumns {
						if _, has := r[c]; !has {
							t.Fatalf("%v: a row without %s", more, c)
						}
						delete(r, c)
					}
					questions = append(questions, r)
				}
				if len(rows) != 9 || len(questions) != 4 {
					t.Fatalf("%v: %d rows, %d questions", more, len(rows), len(questions))
				}
				lg["rows"], _ = json.Marshal(questions)
				b, _ := json.Marshal(lg)
				c, err := canonical.Form(b)
				if err != nil {
					t.Fatal(err)
				}
				want = string(c) + "\n"
			} else {
				recs, err := csv.NewReader(strings.NewReader(all.stdout)).ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				drop := map[int]bool{}
				word := -1
				for i, h := range recs[0] {
					for _, c := range allColumns {
						if h == c {
							drop[i] = true
						}
					}
					if h == "word" {
						word = i
					}
				}
				if len(drop) != len(allColumns) || len(recs) != 10 {
					t.Fatalf("%v: header %v, %d records", more, recs[0], len(recs))
				}
				var buf bytes.Buffer
				w := csv.NewWriter(&buf)
				for i, rec := range recs {
					if i > 0 && rec[word] != "ask" {
						continue
					}
					var out []string
					for j, c := range rec {
						if !drop[j] {
							out = append(out, c)
						}
					}
					_ = w.Write(out)
				}
				w.Flush()
				want = buf.String()
			}
			if plain.stdout != want {
				t.Fatalf("%v %s: without --all\n%s\nwant\n%s", more, format, plain.stdout, want)
			}
		}
	}
	// And the same bytes as --json carries them.
	r := run(t, env(), append(base, "--json")...)
	plain := must(t, base...)
	if r.stdout != `{"exit":0,"log":`+strings.TrimSuffix(plain.stdout, "\n")+"}\n" {
		t.Fatalf("--json: %v", r)
	}
}
