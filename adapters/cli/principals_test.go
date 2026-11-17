package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// TestBindForAPolicy binds with a policy file whose members are in another order than the canonical
// form's, and with spaces: the binding's policy is the typed hash of the policy's canonical bytes,
// computed here from the canonical text written by hand, and the binding founds in a set with that
// policy. A policy file with a repeated member name, or none at all, is error file with the detail
// policy and prints no binding; bind with no --policy is error usage (setup-contract-2, commands-3).
func TestBindForAPolicy(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	canon := `{"principals":[{"principal":"` + a.refs["principal"] + `","roles":["peer"]}],"roles":["peer"]}`
	if b, err := os.ReadFile(filepath.Join(a.dir, "policy.json")); err != nil || string(b) != canon {
		t.Fatalf("the Accord's policy is %q, want %q", b, canon)
	}
	shuffled := filepath.Join(a.dir, "shuffled.json")
	if err := os.WriteFile(shuffled, []byte(`{ "roles": [ "peer" ],
  "principals": [ { "roles": [ "peer" ], "principal": "`+a.refs["principal"]+`" } ] }`), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(canon))
	want := "sha256:" + hex.EncodeToString(sum[:])

	bind := []string{"bind", "--key", a.key("principal"), "--actor", a.refs["ann"], "--role", "peer",
		"--mediator", a.refs["mediator"], "--until", "2099-01-01T00:00:00Z"}
	o := object(t, run(t, env(), append(bind, "--policy", shuffled, "--json")...))
	members(t, o, "exit", "binding")
	var b struct{ Policy string }
	if err := json.Unmarshal(o["binding"], &b); err != nil || b.Policy != want {
		t.Fatalf("the binding names the policy %q, want %q", b.Policy, want)
	}
	plain := must(t, append(bind, "--policy", shuffled)...)
	annBinding := filepath.Join(a.dir, "ann.shuffled.binding")
	if err := os.WriteFile(annBinding, []byte(plain.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	founding := must(t, "founding", "--policy", shuffled, "--mediator", filepath.Join(a.dir, "mediator.json"),
		"--card", filepath.Join(a.dir, "card.json"),
		"--declaration", filepath.Join(a.dir, "ann.declaration"), "--binding", annBinding,
		"--declaration", filepath.Join(a.dir, "bob.declaration"), "--binding", filepath.Join(a.dir, "bob.binding"))
	if l := founding.lines(); len(l) == 0 || l[0] != "accord "+a.id {
		t.Fatalf("the set with the binding made from the shuffled policy founds as %q, want accord %s", l, a.id)
	}

	repeated := filepath.Join(a.dir, "repeated.json")
	if err := os.WriteFile(repeated, []byte(`{"roles":["peer"],"roles":["peer"],"principals":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{repeated, filepath.Join(a.dir, "absent.json")} {
		r := run(t, env(), append(bind, "--policy", path)...)
		if r.code != 3 || r.stdout != "" || r.stderr != "error file policy\n" {
			t.Fatalf("bind --policy %s: %v", filepath.Base(path), r)
		}
		r = run(t, env(), append(bind, "--policy", path, "--json")...)
		if r.code != 3 || r.stdout != `{"detail":["policy"],"error":"file","exit":3}`+"\n" {
			t.Fatalf("bind --policy %s --json: %v", filepath.Base(path), r)
		}
	}
	if r := run(t, env(), append(bind, "--json")...); r.code != 3 || r.stdout != `{"error":"usage","exit":3}`+"\n" {
		t.Fatalf("bind without --policy: %v", r)
	}
}

// TestVerifyNamesWhoStandsBehindEachMember verifies the journal of an Accord whose two members are
// bound by two principals, ann by principal and bob by other-principal: with --json, members lists
// each member's actor, roles and principal in actor-reference order, and without it standard error
// carries one member line per member, in that order, after the module's lines and before the counts
// (commands-4).
func TestVerifyNamesWhoStandsBehindEachMember(t *testing.T) {
	t.Parallel()
	a := newAccordBy(t, map[string]string{"bob": "other-principal"}, "ann", "bob")
	a.start(t)
	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello")...)
	type member struct {
		Actor     string   `json:"actor"`
		Roles     []string `json:"roles"`
		Principal string   `json:"principal"`
	}
	want := []member{
		{a.refs["ann"], []string{"peer"}, a.refs["principal"]},
		{a.refs["bob"], []string{"peer"}, a.refs["other-principal"]},
	}
	if want[1].Actor < want[0].Actor {
		want[0], want[1] = want[1], want[0]
	}
	verify := []string{"verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", filepath.Join(a.dir, "founding.json")}
	o := object(t, run(t, env(), append(verify, "--json")...))
	var got []member
	if err := json.Unmarshal(o["members"], &got); err != nil {
		t.Fatalf("members %s: %v", o["members"], err)
	}
	if len(got) != len(want) {
		t.Fatalf("members %s, want %v", o["members"], want)
	}
	for i := range want {
		if got[i].Actor != want[i].Actor || got[i].Principal != want[i].Principal || !slices.Equal(got[i].Roles, want[i].Roles) {
			t.Fatalf("members %s, want %v", o["members"], want)
		}
	}

	l := must(t, verify...).lines()
	counts := slices.IndexFunc(l, func(s string) bool { return strings.HasPrefix(s, cli.WordPass+" ") })
	if counts < 4 || !strings.HasPrefix(l[counts-4], "module ") || !strings.HasPrefix(l[counts-1], "custody: ") {
		t.Fatalf("verify printed %q", l)
	}
	for i, m := range want {
		if line := "member " + m.Actor + " roles peer principal " + m.Principal; l[counts-3+i] != line {
			t.Fatalf("line %d is %q, want %q", counts-3+i, l[counts-3+i], line)
		}
	}
}
