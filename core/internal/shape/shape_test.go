package shape

import "testing"

// TestTimestamp holds the one timestamp rule: whole seconds, Z, a calendar date, 00:00:00 to
// 23:59:59, no leap second.
func TestTimestamp(t *testing.T) {
	t.Parallel()
	for s, ok := range map[string]bool{
		"2026-11-12T09:00:00Z": true, "2028-02-29T23:59:59Z": true, "2000-02-29T00:00:00Z": true,
		"2026-12-31T23:59:60Z": false, "2026-12-31T24:00:00Z": false, "2027-02-29T00:00:00Z": false,
		"1900-02-29T00:00:00Z": false, "2026-11-12T09:00:00": false, "2026-11-12T09:00:00.5Z": false,
		"2026-11-12T09:00:00+00:00": false, "2026-11-12 09:00:00Z": false, "2026-13-01T00:00:00Z": false,
		"9999-12-31T23:59:59Z": true, "": false,
	} {
		if Timestamp(s) != ok {
			t.Fatalf("Timestamp(%q) = %v", s, !ok)
		}
	}
}

// TestAddSeconds checks a question's deadline: the give's time plus the timeout, and a deadline past
// 9999-12-31T23:59:59Z is that time (maybe-1).
func TestAddSeconds(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		at   string
		n    int64
		want string
	}{
		{"2026-11-12T09:00:00Z", 3600, "2026-11-12T10:00:00Z"},
		{"2026-11-12T09:00:00Z", 1, "2026-11-12T09:00:01Z"},
		{"2028-02-28T23:59:59Z", 1, "2028-02-29T00:00:00Z"},
		{"2026-11-12T09:00:00Z", MaxSafe, LastTime},
		{"9999-12-31T23:59:58Z", 1, LastTime},
		{"9999-12-31T23:59:58Z", 2, LastTime},
	} {
		if got := AddSeconds(c.at, c.n); got != c.want {
			t.Fatalf("AddSeconds(%s, %d) = %s, want %s", c.at, c.n, got, c.want)
		}
	}
}

// TestLabelsAndGates checks the forms of labels, kinds and reserved gates (setup-contract-1,
// crossing-1).
func TestLabelsAndGates(t *testing.T) {
	t.Parallel()
	for s, ok := range map[string]bool{"a": true, "a-b-1": true, "-a": false, "a-": false, "a--b": false, "A": false, "a_b": false, "": false} {
		if Label(s) != ok {
			t.Fatalf("Label(%q) = %v", s, !ok)
		}
	}
	long := "a123456789012345678901234567890123456789012345678901234567890123"
	if !Label(long) || Label(long+"4") {
		t.Fatal("the 64-character limit")
	}
	for s, ok := range map[string]bool{"out_you": true, "out-you": true, "Out_You": false, "out__you": false, "_out": false} {
		if Gate(s) != ok {
			t.Fatalf("Gate(%q) = %v", s, !ok)
		}
	}
	if _, ok := Base64("YR=="); ok {
		t.Fatal("base64 with padding bits set decodes")
	}
	if b, ok := Base64("YQ=="); !ok || string(b) != "a" {
		t.Fatal("YQ== does not decode")
	}
}
