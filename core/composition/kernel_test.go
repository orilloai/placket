package composition_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
)

// TestKernelHash computes the kernel hash again from the module's source on disk: the typed hash of
// {"composition.go": its bytes} (setup-contract-4, docs/adr/0012-the-kernel-hash.md).
func TestKernelHash(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("composition.go")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := json.Marshal(map[string]string{"composition.go": string(src)})
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonical.TypedHash(obj)
	if err != nil {
		t.Fatal(err)
	}
	if got := composition.KernelHash(); got != want {
		t.Fatalf("kernel hash %q, want %q", got, want)
	}
	if composition.KernelVersion != "0.2.0" {
		t.Fatalf("kernel version %q, want 0.2.0", composition.KernelVersion)
	}
	if setup.Spec != "0.1" {
		t.Fatalf("spec %q, want 0.1", setup.Spec)
	}
}

// TestCard is the module's card, byte for byte, and founds: the ask-and-log set with its card
// replaced founds, and its card hash is the typed hash of Card (setup-contract-4).
func TestCard(t *testing.T) {
	t.Parallel()
	want := `{"judges":[],"kernel":{"hash":"` + composition.KernelHash() + `","version":"0.2.0"},"spec":"0.1"}`
	if got := string(composition.Card()); got != want {
		t.Fatalf("card %s, want %s", got, want)
	}
	sealed, foundedAt := ptesting.Founding(t, "ask-and-log")
	var set map[string]json.RawMessage
	if err := json.Unmarshal(ptesting.Clear(t, sealed), &set); err != nil {
		t.Fatal(err)
	}
	set["card"] = composition.Card()
	raw, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ptesting.Open(t, c, foundedAt)
	if err != nil {
		t.Fatalf("found: %v", err)
	}
	h, err := canonical.TypedHash(composition.Card())
	if err != nil {
		t.Fatal(err)
	}
	if a.Card() != h {
		t.Fatalf("card hash %q, want %q", a.Card(), h)
	}
}

// TestCardIsACopy changes what Card returned, and the next Card is unchanged (results-are-copies).
func TestCardIsACopy(t *testing.T) {
	t.Parallel()
	first := composition.Card()
	want := string(first)
	for i := range first {
		first[i] = 'x'
	}
	if got := string(composition.Card()); got != want {
		t.Fatalf("card after a change to a copy: %s, want %s", got, want)
	}
}
