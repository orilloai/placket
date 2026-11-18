package conformance

import (
	"strings"
	"testing"
)

// TestAnswerStepPairing changes answer steps' expectations against their decoded calls: an accepted
// call that expects a decline, a declined call that expects pass ok, a cancelled call that expects a
// decline, and an expected release of a call that asks for none each fail the run as a malformed
// vector, and the vectors as written pass (conformance-vectors-6, U18).
func TestAnswerStepPairing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		vector  string
		step    int
		verdict string
		reason  string
		release bool
	}{
		{"questions/an-accept-that-passes", 1, "block", "declined", false},
		{"questions/a-decline-releases-nothing", 1, "pass", "ok", false},
		{"questions/cancel-by-asker", 1, "block", "declined", false},
		{"questions/a-change-returned-to-the-asker", 1, "pass", "ok", true},
	} {
		v := vectorFile(t, c.vector)
		for _, p := range runParts(t, v) {
			if p.Failure != "" {
				t.Fatalf("%s as written: %s", c.vector, p.Failure)
			}
		}
		e := v["steps"].([]any)[c.step].(map[string]any)["expect"].(map[string]any)
		e["verdict"], e["reason"] = c.verdict, c.reason
		if c.release {
			e["release"] = map[string]any{"verdict": "pass", "reason": "ok"}
		}
		parts := runParts(t, v)
		last := parts[len(parts)-1]
		if !strings.HasPrefix(last.Failure, "a malformed vector: ") || last.Name != "step-"+string(rune('0'+c.step)) {
			t.Fatalf("%s changed: parts %+v", c.vector, parts)
		}
	}
}
