package jev

import (
	"testing"
)

// FuzzReadReply reads any bytes as Jev's reply, seeded from the reply tests' cases: it never panics,
// and a reply it reads has the verdict pass or block and a score, when it has one, from 0 to 100
// (fuzz-parsers).
func FuzzReadReply(f *testing.F) {
	for _, c := range replyCases {
		f.Add([]byte(c.reply))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		a, ok := readReply(b)
		if !ok {
			return
		}
		if (a.Verdict != "pass" && a.Verdict != "block") || (a.Score != nil && (*a.Score < 0 || *a.Score > 100)) {
			t.Fatalf("read %q, %v", a.Verdict, a.Score)
		}
	})
}
