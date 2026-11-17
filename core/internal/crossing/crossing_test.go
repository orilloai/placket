package crossing

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// TestBodyReadsEachForm reads a body given in each form, and refuses a value of the wrong type, a
// raw that is not standard base64, a null data and a form this version has not (crossing-2).
func TestBodyReadsEachForm(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, form string
		v          any
		want       []byte
		ok         bool
	}{
		{"text", "text", "aé", []byte("a\xc3\xa9"), true},
		{"raw", "raw", "AP8=", []byte{0x00, 0xff}, true},
		{"data, canonically", "data", map[string]any{"b": 1.0, "a": "x"}, []byte(`{"a":"x","b":1}`), true},
		{"text not a string", "text", 1.0, nil, false},
		{"raw not a string", "raw", true, nil, false},
		{"raw in another encoding", "raw", "AP9=", nil, false},
		{"null data", "data", nil, nil, false},
		{"url", "url", "https://example.com/", nil, false},
	} {
		got, ok := Body(c.form, c.v)
		if ok != c.ok || !bytes.Equal(got, c.want) {
			t.Fatalf("%s: got %x, %v; want %x, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestSaltReadsLowercaseHex reads a salt of exactly 32 bytes, and refuses one of 16, 31 or 33 bytes,
// uppercase hex, odd length and a value that is not a string (crossing-2).
func TestSaltReadsLowercaseHex(t *testing.T) {
	t.Parallel()
	seq := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i)
		}
		return b
	}
	thirtyTwo := hex.EncodeToString(seq(32))
	for _, c := range []struct {
		name string
		v    any
		want []byte
		ok   bool
	}{
		{"32 bytes", thirtyTwo, seq(32), true},
		{"16 bytes", "000102030405060708090a0b0c0d0e0f", nil, false},
		{"31 bytes", thirtyTwo[:62], nil, false},
		{"33 bytes", thirtyTwo + "20", nil, false},
		{"uppercase", strings.ToUpper(thirtyTwo), nil, false},
		{"odd length", thirtyTwo + "0", nil, false},
		{"not a string", 16.0, nil, false},
	} {
		got, ok := Salt(c.v)
		if ok != c.ok || !bytes.Equal(got, c.want) {
			t.Fatalf("%s: got %x, %v; want %x, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestDeliverHoldsThePartAndItsReceipt writes a delivery: the receipt, its id and the part's
// members, in canonical form; a receipt not in canonical form writes none (crossing-3).
func TestDeliverHoldsThePartAndItsReceipt(t *testing.T) {
	t.Parallel()
	part := map[string]any{"messageId": "m-1", "kind": "note", "salt": "00", "text": "a"}
	got := Deliver(part, []byte(`{"payload":"x"}`), "sha256:00")
	want := `{"kind":"note","messageId":"m-1","receipt":{"payload":"x"},"receiptId":"sha256:00","salt":"00","text":"a"}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if d := Deliver(part, []byte(`{"payload":"x",}`), "sha256:00"); d != nil {
		t.Fatalf("a receipt not in canonical form gave %s", d)
	}
}
