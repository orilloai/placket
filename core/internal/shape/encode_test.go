package shape

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestEncodeWritesCanonicalBytes writes a value that a JSON encoder alone gets wrong in three ways:
// it escapes <, & and U+2028, and it orders names by their UTF-8 bytes, which puts U+FF61 before
// U+1F600. Encode writes them raw, and orders names by UTF-16 code units (canonical-1).
func TestEncodeWritesCanonicalBytes(t *testing.T) {
	t.Parallel()
	v := Object{
		"\uff61":     []any{json.Number("3"), true, nil},
		"\U0001F600": int64(2),
		"<&\u2028":   "a<b&c\u2028d",
	}
	want := []byte("{\"<&\xe2\x80\xa8\":\"a<b&c\xe2\x80\xa8d\",\"\xf0\x9f\x98\x80\":2,\"\xef\xbd\xa1\":[3,true,null]}")
	if got := Encode(v); !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	again, c, err := Decode(want)
	if err != nil || !bytes.Equal(c, want) || !bytes.Equal(Encode(again), want) {
		t.Fatalf("the canonical bytes do not read back: %q, %v", c, err)
	}
}
