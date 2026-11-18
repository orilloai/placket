package local_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// proposed is a world whose window starts the Accord proposed: no acceptances, and a hook that
// keeps acceptances.json's bytes when the window hands them over.
func proposed(t *testing.T) (*world, local.Window, *[]byte) {
	t.Helper()
	w := newWorld(t)
	cfg := w.window(t)
	cfg.Acceptances = nil
	var kept []byte
	cfg.Accepted = func(b []byte) error { kept = append([]byte(nil), b...); return nil }
	return w, cfg, &kept
}

// viewOf is the view the window serves name, which must be name's own.
func (w *world) viewOf(t *testing.T, name string) []byte {
	t.Helper()
	r := w.call(t, name, local.OpView, nil)
	if r.View == nil {
		t.Fatalf("%s's view: %+v", name, r)
	}
	if v := memberOf(t, r.View, "viewer"); v != key(t, name).Reference() {
		t.Fatalf("%s was shown the view of %s", name, v)
	}
	return r.View
}

// acceptOf has name accept the view the window serves it, and returns the reply and the acceptance.
func (w *world) acceptOf(t *testing.T, name string) (local.Reply, []byte) {
	t.Helper()
	acc, err := founding.Accept(w.accord, w.viewOf(t, name), key(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return w.call(t, name, local.OpAccept, local.AcceptArgs(acc)), acc
}

func memberOf(t *testing.T, b []byte, name string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	s, _ := m[name].(string)
	return s
}

// TestAProposedAccordFoundsOnceEveryMemberAccepts is U11 and U12: a window started with no
// acceptances serves only view and accept; a give before every member has accepted is door
// not-founded and writes nothing; each acceptance signs the typed hash of the view the window served,
// under the acceptance's type URI; once the last member accepts, the window hands over
// acceptances.json, journals the founding and emits its founding, and a give passes.
func TestAProposedAccordFoundsOnceEveryMemberAccepts(t *testing.T) {
	t.Parallel()
	w, cfg, kept := proposed(t)
	r := start(t, cfg)
	r.mu.Lock()
	started := (*r.events)[0]
	r.mu.Unlock()
	if started.Kind != local.EventStarted || !started.Proposed || started.Founded || started.Accord != w.accord {
		t.Fatalf("the start: %+v", started)
	}
	if _, err := os.Stat(filepath.Join(w.journal, "receipts.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a proposed Accord wrote its journal: %v", err)
	}

	rep, acc := w.acceptOf(t, "proposer")
	if !rep.IsAccept || rep.Founded {
		t.Fatalf("the first acceptance: %+v", rep)
	}
	// U11: the acceptance names the view's typed hash and verifies under the acceptance's type URI.
	vh, _ := canonical.TypedHash(w.viewOf(t, "proposer"))
	if memberOf(t, acc, "view") != vh || !signing.Verify(acc, signing.TypeAcceptance, key(t, "proposer").Reference()) {
		t.Fatal("the acceptance does not sign the view's typed hash under the acceptance's type URI")
	}
	if g := w.give(t, "proposer", "applier", "n-1", "note", "early"); g.Door != local.NotFounded {
		t.Fatalf("a give before every member accepted: %+v", g)
	}
	for _, op := range []string{local.OpGet, local.OpQuestions} {
		if g := w.call(t, "applier", op, nil); g.Door != local.NotFounded {
			t.Fatalf("a %s before every member accepted: %+v", op, g)
		}
	}
	if len(w.receipts(t)) != 0 || *kept != nil {
		t.Fatal("a proposed Accord wrote something")
	}

	if rep, _ := w.acceptOf(t, "applier"); !rep.IsAccept || rep.Founded {
		t.Fatalf("the second acceptance: %+v", rep)
	}
	rep, _ = w.acceptOf(t, "decider")
	if !rep.IsAccept || !rep.Founded {
		t.Fatalf("the last acceptance: %+v", rep)
	}
	if _, err := founding.Found(founding.Inputs{Set: w.set, Openings: setup.Openings{ByActor: w.openings}, Acceptances: *kept},
		"2026-11-12T09:00:00Z"); err != nil {
		t.Fatalf("acceptances.json does not found the Accord: %v", err)
	}
	if !bytes.Equal(*kept, w.acceptances) {
		t.Fatal("acceptances.json is not every member's acceptance in from order")
	}
	r.mu.Lock()
	var foundedEv *local.Event
	for i := range *r.events {
		if (*r.events)[i].Kind == local.EventFounded {
			foundedEv = &(*r.events)[i]
		}
	}
	r.mu.Unlock()
	if foundedEv == nil || foundedEv.Accord != w.accord {
		t.Fatal("no founding event")
	}
	if g := w.give(t, "proposer", "applier", "n-2", "note", "now"); g.Outcome == nil || g.Outcome.Verdict != "pass" {
		t.Fatalf("a give once founded: %+v", g)
	}
}

// TestARestartWithTheAcceptancesIsFounded is U14: a window founded by its members' acceptances and
// stopped starts again with the acceptances it wrote, founded at once, and serves a give with no new
// acceptance; a founded journal started without them is refused acceptance-missing.
func TestARestartWithTheAcceptancesIsFounded(t *testing.T) {
	t.Parallel()
	w, cfg, kept := proposed(t)
	r := start(t, cfg)
	for _, n := range []string{"proposer", "applier", "decider"} {
		w.acceptOf(t, n)
	}
	r.stop(t)

	without := w.window(t)
	without.Acceptances = nil
	var se *setup.Error
	if err := local.Run(context.Background(), without); !errors.As(err, &se) || se.Code != setup.AcceptanceMissing {
		t.Fatalf("a founded journal with no acceptances: %v", err)
	}

	again := w.window(t)
	again.Acceptances = *kept
	r = start(t, again)
	r.mu.Lock()
	started := (*r.events)[0]
	r.mu.Unlock()
	if started.Proposed || started.Founded {
		t.Fatalf("the restart: %+v", started)
	}
	if g := w.give(t, "proposer", "applier", "n-1", "note", "after"); g.Outcome == nil || g.Outcome.Verdict != "pass" {
		t.Fatalf("a give after the restart: %+v", g)
	}
}

// TestAViewIsOnlyAMembersOwn holds the door's view and accept to a member's own: a stranger's view
// and accept are not-a-member; a member that hands over another member's acceptance, one whose view
// is not its own, or one another key signed is refused with the founding code of the row it fails,
// and nothing is kept.
func TestAViewIsOnlyAMembersOwn(t *testing.T) {
	t.Parallel()
	w, cfg, kept := proposed(t)
	start(t, cfg)
	if r := w.call(t, "stranger", local.OpView, nil); r.Rejection != "not-a-member" {
		t.Fatalf("a stranger's view: %+v", r)
	}
	if r := w.call(t, "stranger", local.OpAccept, local.AcceptArgs([]byte(`{}`))); r.Rejection != "not-a-member" {
		t.Fatalf("a stranger's accept: %+v", r)
	}
	mine, _ := founding.Accept(w.accord, w.viewOf(t, "applier"), key(t, "applier"))
	otherView, _ := founding.Accept(w.accord, w.viewOf(t, "decider"), key(t, "applier"))
	byOther, _ := founding.Accept(w.accord, w.viewOf(t, "applier"), key(t, "decider"))
	byOther = bytes.Replace(byOther, []byte(key(t, "decider").Reference()), []byte(key(t, "applier").Reference()), 1)
	for _, c := range []struct {
		name, who string
		acc       []byte
		code      string
	}{
		{"another member's acceptance", "proposer", mine, string(setup.InvalidShape)},
		{"another member's view", "applier", otherView, string(setup.AcceptanceView)},
		{"another key's signature", "applier", byOther, string(setup.AcceptanceSignature)},
	} {
		if r := w.call(t, c.who, local.OpAccept, local.AcceptArgs(c.acc)); r.Founding != c.code || r.IsAccept {
			t.Fatalf("%s: %+v, want %s", c.name, r, c.code)
		}
	}
	if *kept != nil {
		t.Fatal("a refused acceptance was kept")
	}
}
