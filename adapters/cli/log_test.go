package cli_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// logRows runs log and returns its rows, from JSON or CSV, as maps of the columns.
func logRows(t testing.TB, s *sixRun, format string, kept bool, more ...string) ([]map[string]string, result) {
	t.Helper()
	args := []string{"log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--format", format}
	if kept {
		for _, who := range []string{"proposer", "applier", "decider"} {
			args = append(args, "--kept", s.state(who))
		}
	}
	r := must(t, append(args, more...)...)
	var rows []map[string]string
	if format == "json" {
		var lg struct {
			Accord, Mediator, Claim, NotClaimed string
			Rows                                []map[string]any
		}
		if err := json.Unmarshal([]byte(r.stdout), &lg); err != nil {
			t.Fatal(err)
		}
		if lg.Accord != s.id || lg.Mediator != s.refs["mediator"] || !strings.HasPrefix(lg.Claim, "claim: ") || !strings.HasPrefix(lg.NotClaimed, "not claimed: ") {
			t.Fatalf("log: %s", r.stdout)
		}
		for _, row := range lg.Rows {
			m := map[string]string{}
			for k, v := range row {
				if l, ok := v.([]any); ok {
					var ids []string
					for _, x := range l {
						ids = append(ids, x.(string))
					}
					m[k] = strings.Join(ids, " ")
				} else {
					m[k] = v.(string)
				}
			}
			rows = append(rows, m)
		}
		return rows, r
	}
	recs, err := csv.NewReader(strings.NewReader(r.stdout)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs[1:] {
		m := map[string]string{}
		for i, c := range rec {
			m[recs[0][i]] = c
		}
		rows = append(rows, m)
	}
	return rows, r
}

// copyDir copies a directory to a new one and returns it.
func copyDir(t testing.TB, from string) string {
	t.Helper()
	to := filepath.Join(t.TempDir(), filepath.Base(from))
	if out, err := exec.Command("cp", "-R", from, to).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	return to
}

// TestTheLogOfTheWorkedExample is the log of the worked example's journal and its three state
// directories: one row per proposal, accepted, changed, declined and undeliverable, with the same
// answer and released_at with and without the state directories, in JSON and in CSV; the topic and
// the comment only from kept parts, the comment emptied by --no-words, and text and changed_text
// only under --words. A kept part with one character changed is left out, with a line on stderr;
// and with one byte of the journal changed, log prints no row and exits 3 (log-from-receipts).
func TestTheLogOfTheWorkedExample(t *testing.T) {
	t.Parallel()
	s := six(t)
	for _, format := range []string{"json", "csv"} {
		bare, _ := logRows(t, s, format, false)
		full, r := logRows(t, s, format, true)
		words, _ := logRows(t, s, format, true, "--words")
		quiet, _ := logRows(t, s, format, true, "--no-words")
		if len(bare) != 4 || len(full) != 4 || r.stderr != map[string]string{"json": "", "csv": r.stderr}[format] {
			t.Fatalf("%s: %d and %d rows; stderr %q", format, len(bare), len(full), r.stderr)
		}
		answers := []string{"accepted", "changed", "declined", "undeliverable"}
		for i, row := range full {
			if row["proposal"] != []string{s.accepted, s.changed, s.declined, s.open}[i] || row["answer"] != answers[i] ||
				bare[i]["answer"] != answers[i] || bare[i]["released_at"] != row["released_at"] || row["answerer"] != s.refs["decider"] {
				t.Fatalf("%s row %d: %v / %v", format, i, row, bare[i])
			}
			if (i < 2) != (row["released_at"] != "") {
				t.Fatalf("%s row %d: released_at %q", format, i, row["released_at"])
			}
			if bare[i]["topic"] != "" || bare[i]["comment"] != "" || row["topic"] != "billing" {
				t.Fatalf("%s row %d: topic %q, %q", format, i, bare[i]["topic"], row["topic"])
			}
			if _, has := row["changed_text"]; has {
				t.Fatalf("%s: changed_text without --words", format)
			}
			if quiet[i]["comment"] != "" {
				t.Fatalf("%s: a comment under --no-words", format)
			}
		}
		if full[1]["comment"] != "Someone reads it." || full[2]["comment"] != "Kept seven years." || full[0]["comment"] != "" {
			t.Fatalf("%s: comments %q %q", format, full[1]["comment"], full[2]["comment"])
		}
		if words[1]["changed_text"] != "Keep notes, hidden." || words[0]["text"] != "Rename the column total to amount." || words[0]["changed_text"] != "" {
			t.Fatalf("%s: words %v", format, words)
		}
		if format == "csv" && !strings.HasSuffix(r.stderr, "\nnot claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.\n") {
			t.Fatalf("csv stderr: %q", r.stderr)
		}
	}

	// A kept part with one character changed: the applier's copy of the accepted proposal.
	applier := copyDir(t, s.state("applier"))
	p := filepath.Join(applier, "kept.jsonl")
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), "Rename the column total to amount.", "Rename the column total to amounT.", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	r := must(t, "log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--kept", applier, "--words")
	var lg struct{ Rows []map[string]any }
	_ = json.Unmarshal([]byte(r.stdout), &lg)
	if lg.Rows[0]["text"] != "" || r.stderr != "a kept part of receipt "+lg.Rows[0]["receipts"].([]any)[2].(string)+" is covered by no receipt; left out\n" {
		t.Fatalf("a changed kept part: %v", r)
	}

	// One byte of the journal changed.
	journal := copyDir(t, s.journal())
	p = filepath.Join(journal, "receipts.jsonl")
	b, _ = os.ReadFile(p)
	i := strings.Index(string(b), `"sig":"`) + 7
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	_ = os.WriteFile(p, b, 0o600)
	r = run(t, env(), "log", "--mediator", s.refs["mediator"], "--receipts", journal, "--format", "csv")
	if r.code != 3 || r.stdout != "" || r.stderr != "error verify signature receipts.jsonl line 1\n" {
		t.Fatalf("a changed journal: %v", r)
	}
	r = run(t, env(), "verify", "--mediator", s.refs["mediator"], "--receipts", journal)
	if r.code != 3 || r.stdout != "" || r.stderr != "error verify signature receipts.jsonl line 1\n" {
		t.Fatalf("verify of a changed journal: %v", r)
	}
}
