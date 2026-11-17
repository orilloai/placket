package answer

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

func key(t testing.TB, name string) signing.Key {
	t.Helper()
	seed := sha256.Sum256([]byte("placket vectors: key " + name))
	k, err := signing.NewKey(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	return k
}

var (
	accord   = "sha256:" + strings.Repeat("ab", 32)
	question = "sha256:" + strings.Repeat("cd", 32)
	digest   = strings.Repeat("ef", 32)
)

// TestSignAndRead signs a call of each action and reads it back as it was signed (maybe-11).
func TestSignAndRead(t *testing.T) {
	t.Parallel()
	k := key(t, "decider")
	for _, c := range []Call{
		{Accord: accord, Question: question, Action: Accepted, Digest: digest, Release: true},
		{Accord: accord, Question: question, Action: Changed, Digest: digest, Release: false},
		{Accord: accord, Question: question, Action: Declined},
		{Accord: accord, Question: question, Action: Cancelled},
	} {
		b, err := Sign(c, k)
		if err != nil {
			t.Fatalf("Sign(%s): %v", c.Action, err)
		}
		got, err := Read(b)
		if err != nil {
			t.Fatalf("Read(%s): %v", c.Action, err)
		}
		c.From = k.Reference()
		if got != c {
			t.Fatalf("Read(%s) = %+v, want %+v", c.Action, got, c)
		}
		h, _ := canonical.TypedHash(b)
		if ID(b) != h || !strings.HasPrefix(ID(b), "sha256:") {
			t.Fatalf("ID(%s) = %q, want %q", c.Action, ID(b), h)
		}
	}
}

// TestSignIgnoresFrom signs with the key's reference whatever From held, so From is the signer.
func TestSignIgnoresFrom(t *testing.T) {
	t.Parallel()
	k := key(t, "decider")
	b, err := Sign(Call{Accord: accord, Question: question, Action: Declined, From: key(t, "other").Reference()}, k)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Read(b)
	if err != nil || c.From != k.Reference() {
		t.Fatalf("Read = %+v, %v; want From %s", c, err, k.Reference())
	}
}

// TestSignRefusesABadShape signs no call Read would refuse.
func TestSignRefusesABadShape(t *testing.T) {
	t.Parallel()
	k := key(t, "decider")
	for name, c := range map[string]Call{
		"an unknown action":  {Accord: accord, Question: question, Action: "undeliverable"},
		"an accord not hash": {Accord: "x", Question: question, Action: Declined},
		"a short digest":     {Accord: accord, Question: question, Action: Accepted, Digest: "ab"},
	} {
		var e *Error
		if _, err := Sign(c, k); !errors.As(err, &e) || e.Code != CodeCall {
			t.Errorf("Sign(%s) = %v, want answer-call", name, err)
		}
	}
	var e *Error
	if _, err := Sign(Call{Accord: accord, Question: question, Action: Declined}, signing.Key{}); !errors.As(err, &e) || e.Code != CodeCall {
		t.Errorf("Sign with the zero key = %v, want answer-call", err)
	}
}

// edit decodes a signed call, changes it and encodes it again, sig as it was unless the change
// removes it.
func edit(t *testing.T, b []byte, change func(shape.Object)) []byte {
	t.Helper()
	v, _, err := shape.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	o := v.(shape.Object)
	change(o)
	return shape.Encode(o)
}

// resign signs o again under the key, so only its shape is wrong.
func resign(t *testing.T, o shape.Object, k signing.Key) {
	t.Helper()
	delete(o, "sig")
	sig, err := signing.Sign(shape.Encode(o), TypeURI, k)
	if err != nil {
		t.Fatal(err)
	}
	o["sig"] = sig
}

// TestReadRefusals refuses every defect of maybe-11's call with answer-call: bytes, shape and
// signature (section 6, row 2).
func TestReadRefusals(t *testing.T) {
	t.Parallel()
	k := key(t, "decider")
	accept, err := Sign(Call{Accord: accord, Question: question, Action: Accepted, Digest: digest, Release: true}, k)
	if err != nil {
		t.Fatal(err)
	}
	decline, err := Sign(Call{Accord: accord, Question: question, Action: Declined}, k)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"not JSON":            []byte("{"),
		"not canonical":       append([]byte(" "), accept...),
		"not an object":       []byte(`[1]`),
		"signed by another":   edit(t, accept, func(o shape.Object) { resign(t, o, key(t, "other")) }),
		"from changed":        edit(t, accept, func(o shape.Object) { o["from"] = key(t, "other").Reference() }),
		"digest changed":      edit(t, accept, func(o shape.Object) { o["digest"] = strings.Repeat("00", 32) }),
		"release changed":     edit(t, accept, func(o shape.Object) { o["release"] = false }),
		"no sig":              edit(t, accept, func(o shape.Object) { delete(o, "sig") }),
		"an unlisted member":  edit(t, decline, func(o shape.Object) { o["at"] = "2026-11-12T09:00:00Z"; resign(t, o, k) }),
		"digest on a decline": edit(t, decline, func(o shape.Object) { o["digest"] = digest; resign(t, o, k) }),
		"release on a cancel": edit(t, decline, func(o shape.Object) { o["action"] = "cancelled"; o["release"] = true; resign(t, o, k) }),
		"no release":          edit(t, accept, func(o shape.Object) { delete(o, "release"); resign(t, o, k) }),
		"no digest":           edit(t, accept, func(o shape.Object) { delete(o, "digest"); resign(t, o, k) }),
		"release not bool":    edit(t, accept, func(o shape.Object) { o["release"] = "true"; resign(t, o, k) }),
		"digest uppercase":    edit(t, accept, func(o shape.Object) { o["digest"] = strings.Repeat("EF", 32); resign(t, o, k) }),
		"an unknown action":   edit(t, decline, func(o shape.Object) { o["action"] = "undeliverable"; resign(t, o, k) }),
		"accord not a hash":   edit(t, decline, func(o shape.Object) { o["accord"] = "ab"; resign(t, o, k) }),
		"question not a hash": edit(t, decline, func(o shape.Object) { o["question"] = 7; resign(t, o, k) }),
		"from not a ref":      edit(t, decline, func(o shape.Object) { o["from"] = "decider"; resign(t, o, k) }),
		"sig short":           edit(t, decline, func(o shape.Object) { o["sig"] = "ab" }),
	}
	for name, b := range cases {
		var e *Error
		if _, err := Read(b); !errors.As(err, &e) || e.Code != CodeCall {
			t.Errorf("Read(%s) = %v, want answer-call", name, err)
		}
	}
}

// TestIDOfBytesTheFormRefuses has no id for bytes the canonical form refuses.
func TestIDOfBytesTheFormRefuses(t *testing.T) {
	t.Parallel()
	if got := ID([]byte(`{"a":1,"a":2}`)); got != "" {
		t.Fatalf("ID of a repeated name = %q, want empty", got)
	}
}

// TestTypeURIIsSigningsFourth holds the call's type URI to signing-4's TypeAnswer.
func TestTypeURIIsSigningsFourth(t *testing.T) {
	t.Parallel()
	if TypeURI != "https://placket.tech/answer/0.1" {
		t.Fatalf("TypeURI = %s", TypeURI)
	}
}
