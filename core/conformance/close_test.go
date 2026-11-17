package conformance

import (
	"strconv"
	"strings"
	"testing"
)

// TestCloseStepsCompareExactly runs the close group's scenario vectors as written, then changes one
// expected value of a close step, of a fetch after the close and of a give after it, and checks that
// each vector then fails at that step (conformance-vectors-3, conformance-vectors-5, close-1 to
// close-3).
func TestCloseStepsCompareExactly(t *testing.T) {
	t.Parallel()
	stepOf := func(v map[string]any, i int) map[string]any { return v["steps"].([]any)[i].(map[string]any) }
	expectOf := func(v map[string]any, i int) map[string]any { return stepOf(v, i)["expect"].(map[string]any) }
	closeOf := func(v map[string]any, i int) map[string]any { return expectOf(v, i)["close"].(map[string]any) }
	for _, c := range []struct {
		name   string
		step   int
		change func(v map[string]any)
	}{
		{"close/three-questions-one-past-its-deadline", 3, func(v map[string]any) {
			r := expectOf(v, 3)["receipts"].([]any)
			r[0], r[1] = r[1], r[0]
		}},
		{"close/three-questions-one-past-its-deadline", 3, func(v map[string]any) {
			closeOf(v, 3)["receiptId"] = "sha256:" + strings.Repeat("0", 64)
		}},
		{"close/three-questions-one-past-its-deadline", 3, func(v map[string]any) { stepOf(v, 3)["at"] = "2026-11-12T09:30:01Z" }},
		{"close/every-chain-and-every-delivery-not-fetched", 6, func(v map[string]any) { expectOf(v, 6)["receipts"] = []any{expectOf(v, 0)} }},
		{"close/fetches-after-the-close", 3, func(v map[string]any) { stepOf(v, 3)["expect"] = map[string]any{"none": true} }},
		{"close/a-give-and-an-answer-after-the-close", 2, func(v map[string]any) {
			stepOf(v, 2)["expect"] = map[string]any{"rejection": "envelope-sequence"}
		}},
		{"close/a-give-and-an-answer-after-the-close", 4, func(v map[string]any) {
			stepOf(v, 4)["expect"] = map[string]any{"rejection": "no-open-question"}
		}},
	} {
		v := vectorFile(t, c.name)
		for _, p := range runParts(t, v) {
			if p.Failure != "" || p.Pending != "" {
				t.Fatalf("%s as written: part %s: %s%s", c.name, p.Name, p.Failure, p.Pending)
			}
		}
		c.change(v)
		parts := runParts(t, v)
		if len(parts) <= c.step || parts[c.step].Name != "step-"+strconv.Itoa(c.step) || parts[c.step].Failure == "" {
			t.Fatalf("%s: a changed expectation passed at step %d: %+v", c.name, c.step, parts)
		}
	}
}
