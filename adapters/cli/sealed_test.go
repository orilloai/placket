package cli_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
)

// The terms of the sealed tests' two parties: the guard gives and gets notes, and refuses to give a
// secret, a kind the guest's terms never name; the guest gives and gets notes.
func init() {
	commentTerms["guard"] = `{"terms":[
		{"stance":"want","dir":"give","with":["role:guest"],"select":{"kind":["note"]}},
		{"stance":"want","dir":"get","with":["role:guest"],"select":{"kind":["note"]}},
		{"stance":"no","dir":"give","with":["role:guest"],"select":{"kind":["secret"]}}]}`
	commentTerms["guest"] = `{"terms":[
		{"stance":"want","dir":"give","with":["role:guard"],"select":{"kind":["note"]}},
		{"stance":"want","dir":"get","with":["role:guard"],"select":{"kind":["note"]}}]}`
}

// TestNoPartyButTheOperatorHoldsAnothersTerms is U16 (setup-contract-12 to setup-contract-14): on a
// two-party Accord whose window the guard's side runs, holding every opening, the guest runs every
// command it can, view, accept, give, get, questions and verify; nothing any of them prints, and no
// file in the guest's state directory, holds the guard's declaration, its signature, its salt, its
// opening or the kind only its terms name; the guest's view has no crossing of a kind the guest's
// terms do not select. And no view of the guard built from the clear set, under any salt but the
// guard's own, has the typed hash the guard's acceptance names.
func TestNoPartyButTheOperatorHoldsAnothersTerms(t *testing.T) {
	t.Parallel()
	a := proposedAccord(t, "guard", "guest")
	a.start(t)
	must(t, a.as("guard", "view")...)
	must(t, a.as("guard", "accept")...)

	var out []string
	for _, args := range [][]string{
		a.as("guest", "view"), a.as("guest", "view", "--json"), a.as("guest", "accept"),
		a.as("guest", "give", "--to", a.refs["guard"], "--kind", "note", "--text", "hello"),
		a.as("guard", "give", "--to", a.refs["guest"], "--kind", "note", "--text", "back"),
		a.as("guest", "get"), a.as("guest", "questions"),
		{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding()},
		{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--json"},
	} {
		r := run(t, env(), args...)
		if r.code != 0 {
			t.Fatalf("%v: %v", args, r)
		}
		if args[0] != "give" || !strings.Contains(strings.Join(args, " "), a.key("guard")) {
			out = append(out, r.stdout, r.stderr)
		}
	}

	// What the guard's side holds of its own terms: its opening, its salt, its declaration and its
	// signature, and the kind only it names.
	opening, err := os.ReadFile(filepath.Join(a.openings(), cli.OpeningName(a.refs["guard"])))
	if err != nil {
		t.Fatal(err)
	}
	var op struct {
		Declaration json.RawMessage `json:"declaration"`
		Salt        string          `json:"salt"`
	}
	if err := json.Unmarshal(opening, &op); err != nil {
		t.Fatal(err)
	}
	decl, _ := canonical.Form(op.Declaration)
	sig := memberOf(t, decl, "sig")
	secrets := map[string]string{"opening": string(bytes.TrimSuffix(opening, []byte("\n"))), "declaration": string(decl),
		"signature": sig, "salt": op.Salt, "the kind only the guard names": "secret"}

	for _, o := range out {
		for what, s := range secrets {
			if strings.Contains(o, s) {
				t.Fatalf("the guest was shown the guard's %s: %q", what, o)
			}
		}
	}
	err = filepath.Walk(a.key("guest")+".state", func(p string, fi os.FileInfo, err error) error {
		if err != nil || !fi.Mode().IsRegular() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for what, s := range secrets {
			if strings.Contains(string(b), s) {
				t.Errorf("%s holds the guard's %s", p, what)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The guest's view is of notes alone, the one kind its terms select.
	view, err := os.ReadFile(filepath.Join(a.state("guest"), "view.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Crossings []struct{ Kind string }
		Answers   []json.RawMessage
	}
	if err := json.Unmarshal(view, &v); err != nil || len(v.Crossings) != 2 || len(v.Answers) != 0 {
		t.Fatalf("the guest's view: %s", view)
	}
	for _, c := range v.Crossings {
		if c.Kind != "note" {
			t.Fatalf("the guest's view has a crossing of %s: %s", c.Kind, view)
		}
	}

	// Views of the guard built from the clear set, its rows its own, under any salt but its own.
	acc, err := os.ReadFile(a.acceptances())
	if err != nil {
		t.Fatal(err)
	}
	var list []struct{ From, View string }
	if err := json.Unmarshal(acc, &list); err != nil {
		t.Fatal(err)
	}
	accepted := ""
	for _, x := range list {
		if x.From == a.refs["guard"] {
			accepted = x.View
		}
	}
	set, _ := os.ReadFile(a.founding())
	openings := map[string][]byte{}
	for _, p := range a.parties {
		b, _ := os.ReadFile(filepath.Join(a.openings(), cli.OpeningName(a.refs[p])))
		openings[a.refs[p]] = bytes.TrimSuffix(b, []byte("\n"))
	}
	s, err := setup.Found(bytes.TrimSuffix(set, []byte("\n")), setup.Openings{ByActor: openings}, "2026-11-18T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	real, err := founding.View(s, a.refs["guard"])
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := canonical.TypedHash(real); h != accepted {
		t.Fatal("the guard's acceptance does not name its own view")
	}
	var guestSalt struct{ Salt string }
	_ = json.Unmarshal(openings[a.refs["guest"]], &guestSalt)
	for _, salt := range []string{"", strings.Repeat("00", 32), strings.Repeat("ff", 32), guestSalt.Salt, hex.EncodeToString([]byte(a.id))[:64]} {
		guess := bytes.Replace(real, []byte(`"salt":"`+op.Salt+`"`), []byte(`"salt":"`+salt+`"`), 1)
		if h, _ := canonical.TypedHash(guess); h == accepted || bytes.Equal(guess, real) {
			t.Fatalf("a view under the salt %q has the hash the guard accepted", salt)
		}
	}
}
