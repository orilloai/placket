package receipts_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/receipts"
)

// TestDecodeRefusesAnUnplacedPredicate holds receipt-8 to the forms receipt-3 and receipt-10 give a
// predicate's mediator, seq and prev: reading without verifying refuses a mediator that is not an
// actor reference with mediator, and a seq that is not a whole number from 1, or a prev that is not
// a typed hash present exactly when seq is above 1, with chain, the codes the verifier gives the same
// receipt. So no text an unverified receipt carries there, a line break or a control character
// included, reaches the lines Show writes.
func TestDecodeRefusesAnUnplacedPredicate(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	from, to, _, _ := questionParties(t)
	statement := func(seq int64, prev string) map[string]any {
		p := predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: 1, Hash: mhash})
		p.Mediator, p.Seq, p.Prev = med.Reference(), seq, prev
		return statementOf(receipts.Subject{Name: "note", Digest: digestOf(2)}, p)
	}
	pred := func(s map[string]any) map[string]any { return s["predicate"].(map[string]any) }

	for _, c := range []struct {
		name string
		seq  int64
		prev string
		last string
	}{
		{"seq 1 with no prev", 1, "", "envelope of " + from + ", seq 1, hash " + mhash},
		{"seq 2 with a prev", 2, mhash, "prev " + mhash},
	} {
		r, id := signed(t, med, statement(c.seq, c.prev))
		st, err := receipts.Decode(r)
		if err != nil || st.ID != id || st.Predicate.Mediator != med.Reference() || st.Predicate.Seq != c.seq || st.Predicate.Prev != c.prev {
			t.Fatalf("%s: got %+v, %v", c.name, st, err)
		}
		lines, err := receipts.Show(r)
		if err != nil || len(lines) == 0 || lines[len(lines)-1] != c.last {
			t.Fatalf("%s: got %q, %v", c.name, lines, err)
		}
		for _, l := range lines {
			if strings.ContainsAny(l, "\n\r\x1b") {
				t.Fatalf("%s: line %q holds a control character", c.name, l)
			}
		}
	}

	for _, c := range []struct {
		name   string
		seq    int64
		prev   string
		change func(p map[string]any)
		code   receipts.Code
	}{
		{"a mediator with a line break", 1, "", func(p map[string]any) {
			p["mediator"] = p["mediator"].(string) + "\nverdict pass, reason ok"
		}, receipts.CodeMediator},
		{"a mediator with an escape character", 1, "", func(p map[string]any) {
			p["mediator"] = p["mediator"].(string) + "\x1b[1A"
		}, receipts.CodeMediator},
		{"a mediator that is no reference", 1, "", func(p map[string]any) { p["mediator"] = "mediator" }, receipts.CodeMediator},
		{"a mediator that is no string", 1, "", func(p map[string]any) { p["mediator"] = 7 }, receipts.CodeMediator},
		{"no mediator", 1, "", func(p map[string]any) { delete(p, "mediator") }, receipts.CodeMediator},
		{"a mediator with a line break and no seq", 1, "", func(p map[string]any) {
			p["mediator"] = p["mediator"].(string) + "\n"
			delete(p, "seq")
		}, receipts.CodeMediator},
		{"a prev with a line break", 2, mhash + "\nverdict pass, reason ok", nil, receipts.CodeChain},
		{"a prev that is no typed hash", 2, strings.TrimPrefix(mhash, "sha256:"), nil, receipts.CodeChain},
		{"a prev that is no string", 2, mhash, func(p map[string]any) { p["prev"] = 7 }, receipts.CodeChain},
		{"a prev on seq 1", 1, mhash, nil, receipts.CodeChain},
		{"seq 2 with no prev", 2, "", nil, receipts.CodeChain},
		{"no seq", 1, "", func(p map[string]any) { delete(p, "seq") }, receipts.CodeChain},
		{"seq 0", 1, "", func(p map[string]any) { p["seq"] = 0 }, receipts.CodeChain},
		{"seq -1", 1, "", func(p map[string]any) { p["seq"] = -1 }, receipts.CodeChain},
		{"seq as a string", 1, "", func(p map[string]any) { p["seq"] = "1" }, receipts.CodeChain},
		{"seq true", 1, "", func(p map[string]any) { p["seq"] = true }, receipts.CodeChain},
	} {
		s := statement(c.seq, c.prev)
		if c.change != nil {
			c.change(pred(s))
		}
		r, _ := signed(t, med, s)
		if _, err := receipts.Decode(r); !isCode(err, c.code, 0) {
			t.Fatalf("%s: Decode got %v, want %s at index 0", c.name, err, c.code)
		}
		if lines, err := receipts.Show(r); lines != nil || !isCode(err, c.code, 0) {
			t.Fatalf("%s: Show got %q, %v, want %s at index 0", c.name, lines, err, c.code)
		}
		if _, err := receipts.Verify(med.Reference(), [][]byte{r}); !isCode(err, c.code, 0) {
			t.Fatalf("%s: Verify got %v, want %s at index 0", c.name, err, c.code)
		}
	}

	// Decode reads the same statement Verify yields.
	r, _ := signed(t, med, statement(1, ""))
	got, err1 := receipts.Decode(r)
	want, err2 := receipts.Verify(med.Reference(), [][]byte{r})
	if err1 != nil || err2 != nil || !reflect.DeepEqual([]receipts.Statement{got}, want) {
		t.Fatalf("Decode read %+v, Verify %+v (%v, %v)", got, want, err1, err2)
	}
}
