package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
)

// otherHash is a kernel hash no build has, and otherCard a card of another module: this build's
// version under that hash.
var (
	otherHash = "sha256:" + strings.Repeat("0", 64)
	otherCard = `{"spec":"0.1","kernel":{"version":"0.1.0","hash":"` + otherHash + `"},"judges":[]}`
)

// otherLine is the line that names a card of another module (docs/adr/0012-the-kernel-hash.md).
func otherLine(version, hash string) string {
	return "module " + version + " " + hash + " is not this build's decision module, " + composition.KernelVersion + " " + composition.KernelHash()
}

// TestCardDefault prints the binary's own card, and its card hash and module on standard error
// (docs/adr/0012-the-kernel-hash.md).
func TestCardDefault(t *testing.T) {
	t.Parallel()
	r := run(t, env(), "card", "--default")
	want := `{"judges":[],"kernel":{"hash":"` + composition.KernelHash() + `","version":"0.2.0"},"spec":"0.1"}`
	h, err := canonical.TypedHash([]byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if r.code != 0 || r.stdout != want+"\n" || r.stderr != "card "+h+"\nmodule 0.2.0 "+composition.KernelHash()+"\n" {
		t.Fatalf("card --default: %v", r)
	}
}

// TestCardWithoutDefault is a usage error: no other card is built by the command line.
func TestCardWithoutDefault(t *testing.T) {
	t.Parallel()
	r := run(t, env(), "card")
	if r.code != 3 || r.stdout != "" || r.stderr != "usage: placket card: --default is required\nerror usage\n" {
		t.Fatalf("card: %v", r)
	}
	r = run(t, env(), "card", "--default", "extra")
	if r.code != 3 || r.stdout != "" || r.stderr != "usage: placket card: bad flags\nerror usage\n" {
		t.Fatalf("card --default extra: %v", r)
	}
}

// refound founds the Accord's parties again under card, and returns the founding command's result
// and the set's path.
func (a *accord) refound(t *testing.T, card string) (result, string) {
	t.Helper()
	cardPath := filepath.Join(a.dir, "other-card.json")
	if err := os.WriteFile(cardPath, []byte(card), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"founding", "--policy", filepath.Join(a.dir, "policy.json"), "--mediator", filepath.Join(a.dir, "mediator.json"), "--card", cardPath}
	decls, _ := filepath.Glob(filepath.Join(a.dir, "*.declaration"))
	for _, d := range decls {
		args = append(args, "--declaration", d, "--binding", strings.TrimSuffix(d, ".declaration")+".binding")
	}
	r := must(t, args...)
	set := filepath.Join(a.dir, "other-founding.json")
	if err := os.WriteFile(set, []byte(r.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	return r, set
}

// TestFoundingNamesAnotherModule founds a set whose card names another module, and says so after the
// card line; a set of this build's card prints the two lines it always did.
func TestFoundingNamesAnotherModule(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	r, _ := a.refound(t, otherCard)
	l := r.lines()
	if len(l) != 3 || !strings.HasPrefix(l[0], "accord sha256:") || !strings.HasPrefix(l[1], "card sha256:") || l[2] != otherLine("0.1.0", otherHash) {
		t.Fatalf("founding under another module's card: %v", r)
	}
	r, _ = a.refound(t, must(t, "card", "--default").stdout)
	if l := r.lines(); len(l) != 2 || l[0] != "accord "+a.id || l[1] != "card "+a.card {
		t.Fatalf("founding under this build's card: %v", r)
	}
}

// TestWindowRefusesAnotherModule refuses to start a window whose card names another module: it
// names the module, prints tool error founding kernel, and makes no journal.
func TestWindowRefusesAnotherModule(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, card, version, hash string }{
		{"another hash", otherCard, "0.1.0", otherHash},
		{"another version", strings.Replace(must(t, "card", "--default").stdout, `"0.2.0"`, `"0.3.0"`, 1), "0.3.0", composition.KernelHash()},
	} {
		a := newAccord(t, "p1", "p2")
		_, set := a.refound(t, c.card)
		r := run(t, env(), "window", "--key", a.key("mediator"), "--founding", set, "--socket", a.socket(), "--journal", a.journal())
		if r.code != 3 || r.stdout != "" || r.stderr != otherLine(c.version, c.hash)+"\nerror founding kernel\n" {
			t.Fatalf("%s: window: %v", c.name, r)
		}
		if _, err := os.Stat(a.journal()); !os.IsNotExist(err) {
			t.Fatalf("%s: the refused window made its journal: %v", c.name, err)
		}
	}
}

// TestWindowLeavesAnUnreadableCardToFounding leaves a set whose card it cannot read to founding,
// which refuses it by its own code.
func TestWindowLeavesAnUnreadableCardToFounding(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	for _, c := range []string{`{"card":7}`, `{"card":{"spec":"0.1"}}`, `{"card":{"kernel":null}}`} {
		set := filepath.Join(a.dir, "bad.json")
		if err := os.WriteFile(set, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
		r := run(t, env(), "window", "--key", a.key("mediator"), "--founding", set, "--socket", a.socket(), "--journal", a.journal())
		if r.code != 3 || r.stdout != "" || r.stderr != "error founding invalid-shape\n" {
			t.Fatalf("window under %s: %v", c, r)
		}
	}
}

// TestVerifyNamesAnotherModule verifies a journal with no receipts yet under a founding set whose
// card names another module than this build's: the module line is followed by the line that names it another build's.
func TestVerifyNamesAnotherModule(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	_, set := a.refound(t, otherCard)
	if err := os.MkdirAll(a.journal(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.journal(), "receipts.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := must(t, "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", set)
	l := r.lines()
	for i := range l {
		if l[i] == "module 0.1.0 "+otherHash {
			if i+1 < len(l) && l[i+1] == otherLine("0.1.0", otherHash) {
				return
			}
			break
		}
	}
	t.Fatalf("verify: %v", r)
}

// TestVerifyOfThisModule prints no such line when the card is this build's.
func TestVerifyOfThisModule(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "one")...)
	r := must(t, "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", filepath.Join(a.dir, "founding.json"))
	l := r.lines()
	if len(l) < 5 || l[2] != "module 0.2.0 "+composition.KernelHash() || strings.HasPrefix(l[3], "module ") {
		t.Fatalf("verify: %v", r)
	}
}
