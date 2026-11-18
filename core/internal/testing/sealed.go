package ptesting

import (
	"crypto/sha256"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// SaltOf is the seal salt a test gives actor's declaration: the SHA-256 of "placket core tests: salt
// " and the actor's reference.
func SaltOf(actor string) []byte {
	s := sha256.Sum256([]byte("placket core tests: salt " + actor))
	return s[:]
}

// Sealed seals a set a test wrote with its declarations in the clear (setup-contract-12): each
// declaration becomes its seal entry under SaltOf its actor, and its opening is given under the
// actor. When rows 1 to 17 found the sealed set at at, each member whose key is among keys accepts
// its view, and the inputs hold acceptances.json; otherwise they hold no acceptances. It fails the
// test only when the clear set cannot be read, so a test can seal a set founding will refuse.
func Sealed(t testing.TB, clear []byte, at string, keys ...signing.Key) founding.Inputs {
	t.Helper()
	v, _, err := shape.Decode(clear)
	set, ok := v.(shape.Object)
	if err != nil || !ok {
		t.Fatalf("the clear set does not read: %v", err)
	}
	in := founding.Inputs{Openings: setup.Openings{ByActor: map[string][]byte{}}}
	if l, ok := set["declarations"].([]any); ok {
		entries := make([]any, 0, len(l))
		for _, x := range l {
			d, ok := x.(shape.Object)
			actor, _ := d["actor"].(string)
			if !ok {
				entries = append(entries, x)
				continue
			}
			e, o, err := setup.Seal(shape.Encode(d), SaltOf(actor))
			if err != nil {
				entries = append(entries, x)
				continue
			}
			ev, _, _ := shape.Decode(e)
			entries = append(entries, ev)
			in.Openings.ByActor[actor] = o
		}
		set["declarations"] = entries
	}
	in.Set = shape.Encode(set)
	a, err := setup.Found(in.Set, in.Openings, at)
	if err != nil {
		return in
	}
	byRef := map[string]signing.Key{}
	for _, k := range keys {
		byRef[k.Reference()] = k
	}
	var list [][]byte
	for _, m := range a.Members() {
		k, ok := byRef[m]
		if !ok {
			continue
		}
		view, err := founding.View(a, m)
		if err != nil {
			t.Fatalf("the view of a member: %v", err)
		}
		acc, err := founding.Accept(a.ID(), view, k)
		if err != nil {
			t.Fatalf("an acceptance: %v", err)
		}
		list = append(list, acc)
	}
	if in.Acceptances, err = founding.Acceptances(list); err != nil {
		t.Fatalf("the acceptances: %v", err)
	}
	return in
}

// ID is the Accord id of the inputs' set: its typed hash (setup-contract-5).
func ID(t testing.TB, in founding.Inputs) string {
	t.Helper()
	id, err := canonical.TypedHash(in.Set)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Clear is the inputs' set with each seal entry replaced by the declaration its opening holds, so a
// test can change a declaration and seal the set again (Sealed, Open).
func Clear(t testing.TB, in founding.Inputs) []byte {
	t.Helper()
	v, _, err := shape.Decode(in.Set)
	set, ok := v.(shape.Object)
	if err != nil || !ok {
		t.Fatalf("the set does not read: %v", err)
	}
	for i, x := range set["declarations"].([]any) {
		e := x.(shape.Object)
		ov, _, err := shape.Decode(in.Openings.ByActor[e["actor"].(string)])
		if err != nil {
			t.Fatalf("the opening of %v does not read", e["actor"])
		}
		set["declarations"].([]any)[i] = ov.(shape.Object)["declaration"]
	}
	return shape.Encode(set)
}

// Open seals a set written with its declarations in the clear and runs rows 1 to 17 over it at at,
// with every opening (setup-contract-12, setup-contract-13).
func Open(t testing.TB, clear []byte, at string) (*setup.Accord, error) {
	t.Helper()
	in := Sealed(t, clear, at)
	return setup.Found(in.Set, in.Openings, at)
}

// Every is the vectors' keys a test's sets name, for SealedByAll.
var every = []string{"mediator", "operator", "principal", "other-principal", "other-mediator", "proposer", "applier", "decider", "lead", "stranger"}

// SealedByAll is Sealed with every member accepting whose key is one of the vectors' keys.
func SealedByAll(t testing.TB, clear []byte, at string) founding.Inputs {
	t.Helper()
	var keys []signing.Key
	for _, n := range every {
		keys = append(keys, Key(t, n))
	}
	return Sealed(t, clear, at, keys...)
}
