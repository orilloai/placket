package cli_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The terms of the comment tests' parties: a poser asks the taker under a maybe term resolved by
// the judge, and takes an answer from the judge and a comment from the aide, never a comment from
// the judge.
var commentTerms = map[string]string{
	"poser": `{"terms":[
		{"stance":"maybe","dir":"give","with":["role:taker"],"select":{"kind":["proposal"]},"resolve":{"to":["role:judge"],"timeout":3600}},
		{"stance":"want","dir":"get","with":["role:judge"],"select":{"kind":["answer"]}},
		{"stance":"want","dir":"get","with":["role:aide"],"select":{"kind":["comment"]}}]}`,
	"taker": `{"terms":[{"stance":"want","dir":"get","with":["role:poser"],"select":{"kind":["proposal"]}}]}`,
	"judge": `{"terms":[{"stance":"want","dir":"give","with":["role:poser"],"select":{"kind":["answer","comment"]}}]}`,
	"aide":  `{"terms":[{"stance":"want","dir":"give","with":["role:poser"],"select":{"kind":["comment"]}}]}`,
}

// logOf runs log as JSON over the journal and the state directories, and returns its rows and what
// it printed on standard error.
func logOf(t testing.TB, a *accord, kept ...string) ([]map[string]any, string) {
	t.Helper()
	args := []string{"log", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--format", "json"}
	for _, k := range kept {
		args = append(args, "--kept", k)
	}
	r := must(t, args...)
	var lg struct{ Rows []map[string]any }
	if err := json.Unmarshal([]byte(r.stdout), &lg); err != nil {
		t.Fatalf("log: %v", r)
	}
	return lg.Rows, r.stderr
}

// ids is a row's receipts.
func ids(row map[string]any) []string {
	var out []string
	l, _ := row["receipts"].([]any)
	for _, x := range l {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// changeGiven copies a state directory and, in the copy's kept.jsonl, changes old to new inside the
// message of the first part the actor gave that holds old, leaving every other line as it was.
func changeGiven(t testing.TB, dir, old, new string) string {
	t.Helper()
	to := copyDir(t, dir)
	p := filepath.Join(to, "kept.jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, k := range keptLines(t, to) {
		m64 := str(k["message"])
		msg, err := base64.StdEncoding.DecodeString(m64)
		if changed || m64 == "" || err != nil || !strings.Contains(string(msg), old) {
			continue
		}
		b = []byte(strings.Replace(string(b), `"`+m64+`"`, `"`+base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(msg), old, new, 1)))+`"`, 1))
		changed = true
	}
	if !changed {
		t.Fatalf("no given part holds %q", old)
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return to
}

// TestAChangedGivenPartIsLeftOut changes one byte inside the message of a part a party gave and
// kept: the proposer's topic of the accepted proposal, and the decider's comment on the declined
// one. The message no longer matches its receipt's envelope hash, so the log leaves that cell empty,
// keeps every other row's, and says on stderr which receipt's part it left out (log-from-receipts).
func TestAChangedGivenPartIsLeftOut(t *testing.T) {
	t.Parallel()
	s := six(t)
	all := []string{s.state("proposer"), s.state("applier"), s.state("decider")}
	full, stderr := logOf(t, s.accord, all...)
	if len(full) != 4 || stderr != "" || full[0]["topic"] != "billing" || full[2]["comment"] != "Kept seven years." {
		t.Fatalf("the log as kept: %v, %q", full, stderr)
	}

	// The topic of the accepted proposal, billing to billinG.
	rows, stderr := logOf(t, s.accord, changeGiven(t, s.state("proposer"), `"topic":"billing"`, `"topic":"billinG"`), all[1], all[2])
	if stderr != "a kept part of receipt "+s.accepted+" is covered by no receipt; left out\n" {
		t.Fatalf("a changed topic: stderr %q", stderr)
	}
	if len(rows) != 4 || rows[0]["topic"] != "" || rows[1]["topic"] != "billing" || rows[2]["topic"] != "billing" || rows[3]["topic"] != "billing" {
		t.Fatalf("a changed topic: %v", rows)
	}

	// The text of the comment on the declined proposal, one character changed.
	rows, stderr = logOf(t, s.accord, all[0], all[1], changeGiven(t, s.state("decider"), "Kept seven years.", "Kept seven yearS."))
	was, now := ids(full[2]), ids(rows[2])
	if len(rows) != 4 || len(now) != len(was)-1 || strings.Join(now, " ") != strings.Join(was[:len(was)-1], " ") {
		t.Fatalf("a changed comment: receipts %v, then %v", was, now)
	}
	if stderr != "a kept part of receipt "+was[len(was)-1]+" is covered by no receipt; left out\n" {
		t.Fatalf("a changed comment: stderr %q", stderr)
	}
	if rows[2]["comment"] != "" || rows[1]["comment"] != "Someone reads it." || rows[2]["topic"] != "billing" {
		t.Fatalf("a changed comment: %v", rows)
	}
}

// TestOnlyTheAnswerersCrossedCommentIsTheRows asks one question, which the judge accepts. Then the
// aide, a member who is not the answerer, gives the asker a comment that crosses, and the judge
// gives the asker one that is blocked, each with re the question and kept. Neither fills the row's
// comment, from either state directory, and neither is noted, since a receipt covers each.
func TestOnlyTheAnswerersCrossedCommentIsTheRows(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "poser", "taker", "judge", "aide")
	a.start(t)
	r := run(t, env(), a.as("poser", "give", "--to", a.refs["taker"], "--kind", "proposal", "--text", "Rename the column total to amount.", "--topic", "billing", "--keep")...)
	if r.code != 4 {
		t.Fatalf("the proposal: %v", r)
	}
	q := strings.TrimSpace(r.stdout[strings.LastIndex(strings.TrimSuffix(r.stdout, "\n"), "\n")+1:])
	must(t, a.as("judge", "answer", "--question", q)...)
	r = run(t, env(), a.as("aide", "give", "--to", a.refs["poser"], "--kind", "comment", "--text", "From the aide.", "--re", q, "--keep")...)
	if r.code != 0 || !strings.HasPrefix(r.stderr, "pass ok ") {
		t.Fatalf("the aide's comment: %v", r)
	}
	r = run(t, env(), a.as("judge", "give", "--to", a.refs["poser"], "--kind", "comment", "--text", "From the judge.", "--re", q, "--keep")...)
	if r.code != 1 || !strings.HasPrefix(r.stderr, "block ") {
		t.Fatalf("the judge's comment: %v", r)
	}
	for _, who := range []string{"aide", "judge"} {
		if n := len(keptLines(t, a.state(who))); n != 1 {
			t.Fatalf("%s kept %d parts", who, n)
		}
		rows, stderr := logOf(t, a, a.state("poser"), a.state(who))
		if len(rows) != 1 || stderr != "" {
			t.Fatalf("with the %s's state: %v, %q", who, rows, stderr)
		}
		if rows[0]["proposal"] != q || rows[0]["answer"] != "accepted" || rows[0]["answerer"] != a.refs["judge"] || rows[0]["topic"] != "billing" {
			t.Fatalf("with the %s's state: %v", who, rows[0])
		}
		if rows[0]["comment"] != "" || len(ids(rows[0])) != 3 {
			t.Fatalf("with the %s's state, a comment: %q, receipts %v", who, rows[0]["comment"], ids(rows[0]))
		}
	}
}
