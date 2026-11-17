package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func (s *sixRun) reconcile(t testing.TB, kept ...string) result {
	t.Helper()
	args := []string{"reconcile", "--mediator", s.refs["mediator"], "--accord", s.id, "--card", s.card, "--receipts", s.journal()}
	for _, k := range kept {
		args = append(args, "--kept", k)
	}
	return must(t, args...)
}

// TestReconcileVerifyAndCancel reconciles each actor of the worked example after its last get and
// finds nothing; then a delivered line removed is unrecorded, a pass to the applier that no journal
// line holds is removed, and an envelope no receipt echoes is missing. A cancel by an answerer closes
// a question with one receipt, and by its asker is not-an-answerer, exit 2. verify --founding with
// the founding set prints the operator and the module, and with another Accord's fails founding.
func TestReconcileVerifyAndCancel(t *testing.T) {
	t.Parallel()
	s := six(t)
	for _, who := range []string{"proposer", "applier", "decider"} {
		if r := s.reconcile(t, s.state(who)); r.stdout != `{"missing":[],"removed":[],"unrecorded":[]}`+"\n" || r.stderr != "nothing to reconcile\n" {
			t.Fatalf("%s: %v", who, r)
		}
	}
	applier := copyDir(t, s.state("applier"))
	p := filepath.Join(applier, "delivered.jsonl")
	b, _ := os.ReadFile(p)
	lines := strings.SplitAfter(string(b), "\n")
	_ = os.WriteFile(p, []byte(strings.Join(lines[1:], "")), 0o600)
	first, _ := receipts.Decode([]byte(strings.TrimSpace(lines[0])))
	if r := s.reconcile(t, applier); r.stderr != "unrecorded "+s.refs["applier"]+" "+first.ID+"\n" {
		t.Fatalf("a delivered line removed: %v", r)
	}

	applier = copyDir(t, s.state("applier"))
	k, err := keyfile.Read(s.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	pass, v, err := receipts.Build(receipts.Subject{Name: "proposal", Digest: strings.Repeat("ab", 32)}, receipts.Predicate{
		Accord: s.id, Card: s.card, Kind: receipts.KindCrossing, From: s.refs["proposer"], To: s.refs["applier"],
		Seq: 1, At: "2026-11-12T09:00:00Z", Envelope: receipts.Envelope{Sender: s.refs["proposer"], Seq: 50, Hash: "sha256:" + strings.Repeat("cd", 32)},
		Verdict: "pass", Reason: "ok",
	}, k)
	if err != nil {
		t.Fatal(err)
	}
	id := v.Statement().ID
	f, _ := os.OpenFile(filepath.Join(applier, "delivered.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.Write(append(pass, '\n'))
	_ = f.Close()
	if r := s.reconcile(t, applier); r.stderr != "removed "+s.refs["applier"]+" "+id+"\n" {
		t.Fatalf("a delivered receipt no journal holds: %v", r)
	}

	proposer := copyDir(t, s.state("proposer"))
	f, _ = os.OpenFile(filepath.Join(proposer, "envelopes.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"hash":"sha256:` + strings.Repeat("ef", 32) + `","seq":99}` + "\n")
	_ = f.Close()
	if r := s.reconcile(t, proposer); r.stderr != "missing "+s.refs["proposer"]+" seq 99\n" {
		t.Fatalf("an envelope no receipt echoes: %v", r)
	}

	q := s.openOne(t, "Cancel me.")
	before := strings.Count(string(s.receipts(t)), "\n")
	if r := run(t, env(), s.as("proposer", "cancel", "--question", q)...); r.code != 2 || r.stderr != "rejected not-an-answerer\n" {
		t.Fatalf("the asker's cancel: %v", r)
	}
	r := must(t, s.as("decider", "cancel", "--question", q)...)
	if !strings.HasPrefix(r.stderr, "block cancelled sha256:") || strings.Count(string(s.receipts(t)), "\n") != before+1 {
		t.Fatalf("the decider's cancel: %v", r)
	}

	r = must(t, "verify", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--founding", filepath.Join(s.dir, "founding.json"))
	l := r.lines()
	if len(l) != 12 || l[2] != "operator "+s.refs["operator"]+" runs process" || l[3] != "module 0.1.0 "+composition.KernelHash() ||
		l[7] != "pass 6 block 3 ask 5" || l[8] != "questions closed 5 open 0" || l[9] != "releases 2 blocked releases 0" {
		t.Fatalf("verify --founding: %q", l)
	}
	// One member line each, in actor-reference order, all under the one principal.
	var members []string
	for _, who := range []string{"proposer", "applier", "decider"} {
		members = append(members, "member "+s.refs[who]+" roles "+who+" principal "+s.refs["principal"])
	}
	sort.Strings(members)
	if !slices.Equal(l[4:7], members) {
		t.Fatalf("verify --founding members: %q, want %q", l[4:7], members)
	}
	other := newAccord(t, "proposer", "applier", "decider")
	if r := run(t, env(), "verify", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--founding", filepath.Join(other.dir, "founding.json")); r.code != 3 || r.stderr != "error founding\n" {
		t.Fatalf("verify with another Accord's founding set: %v", r)
	}
}
