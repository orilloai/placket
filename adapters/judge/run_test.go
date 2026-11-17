package judge_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/setup"
)

// A fake is a judge a test scripts: it answers answer, or fails with err; with hold set, it waits
// until hold is closed or its context is done, and with that context done it returns its error.
type fake struct {
	answer judge.Answer
	err    error
	hold   chan struct{}
	calls  atomic.Int32
	mu     sync.Mutex
	got    []judge.Request
}

func (f *fake) Judge(ctx context.Context, r judge.Request) (judge.Answer, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.got = append(f.got, r)
	f.mu.Unlock()
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return judge.Answer{}, ctx.Err()
		}
	}
	return f.answer, f.err
}

func score(n int) *int { return &n }

func says(verdict string, s *int) *fake {
	return &fake{answer: judge.Answer{Verdict: verdict, Score: s}}
}

func configured(name string, use setup.Use, threshold int, j judge.Judge) judge.Configured {
	return judge.Configured{Config: judge.Config{Name: name, Provider: "jev", Host: "h.example", Model: "m",
		Instruction: "the instruction of " + name, Threshold: threshold}, Use: use, Judge: j}
}

var request = judge.Request{Kind: "report", Body: []byte("A report.")}

// TestRunStopsAtThePassOrBlock runs a stack of three decide judges: a says none, b blocks, and c is
// never asked; with every judge saying none, the judgment is nil (composition-7). Each judge is asked
// with its own config's instruction.
func TestRunStopsAtThePassOrBlock(t *testing.T) {
	t.Parallel()
	a, b, c := says("abstain", nil), says("block", score(90)), says("pass", nil)
	s := judge.Stack{configured("a", setup.Decide, 0, a), configured("b", setup.Decide, 0, b), configured("c", setup.Decide, 0, c)}
	j, recs := judge.Run(context.Background(), s, request, time.Second)
	if !reflect.DeepEqual(j, &composition.Judgment{Name: "b", Verdict: composition.Block}) || c.calls.Load() != 0 {
		t.Fatalf("Run = %+v, c asked %d times", j, c.calls.Load())
	}
	want := []judge.Record{{Judge: "a", Use: "decide", Outcome: "none", Abstained: true},
		{Judge: "b", Use: "decide", Outcome: "block", Score: score(90)}}
	if !reflect.DeepEqual(recs, want) {
		t.Fatalf("records %#v", lines(recs))
	}
	if got := b.got[0]; got.Kind != "report" || got.Instruction != "the instruction of b" || string(got.Body) != "A report." {
		t.Fatalf("b was asked kind %q, instruction %q", got.Kind, got.Instruction)
	}
	none := judge.Stack{configured("a", setup.Decide, 0, says("abstain", nil)), configured("b", setup.Decide, 0, says("abstain", nil)),
		configured("c", setup.Decide, 0, says("abstain", nil))}
	if j, recs := judge.Run(context.Background(), none, request, time.Second); j != nil || len(recs) != 3 {
		t.Fatalf("every judge none: %+v, %d records", j, len(recs))
	}
}

// TestRunThreshold is the threshold: a pass whose score is below it abstains, at it passes, and with
// no score the label stands.
func TestRunThreshold(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name      string
		answer    *fake
		outcome   string
		abstained bool
	}{
		{"79", says("pass", score(79)), "none", true},
		{"80", says("pass", score(80)), "pass", false},
		{"no score", says("pass", nil), "pass", false},
		{"a block at 10", says("block", score(10)), "none", true},
	} {
		_, recs := judge.Run(context.Background(), judge.Stack{configured("a", setup.Decide, 80, c.answer)}, request, time.Second)
		if len(recs) != 1 || recs[0].Outcome != c.outcome || recs[0].Abstained != c.abstained || recs[0].Error != "" {
			t.Fatalf("%s: %s", c.name, lines(recs))
		}
	}
}

// TestRunErrorsAreNone turns every error into none, with its code: a *judge.Error's own, any other
// error unreachable, an answer with a verdict the runner does not know unparsable, and a judge the
// budget ended timeout, whatever it returned. The give is then asked.
func TestRunErrorsAreNone(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		j    *fake
		code judge.Code
	}{
		{"no key", &fake{err: &judge.Error{Code: judge.CodeNoKey}}, judge.CodeNoKey},
		{"unreachable", &fake{err: &judge.Error{Code: judge.CodeUnreachable}}, judge.CodeUnreachable},
		{"a status", &fake{err: &judge.Error{Code: judge.CodeStatus}}, judge.CodeStatus},
		{"a redirect", &fake{err: &judge.Error{Code: judge.CodeRedirect}}, judge.CodeRedirect},
		{"too large", &fake{err: &judge.Error{Code: judge.CodeTooLarge}}, judge.CodeTooLarge},
		{"unparsable", &fake{err: &judge.Error{Code: judge.CodeUnparsable}}, judge.CodeUnparsable},
		{"another error", &fake{err: errors.New("dial: refused")}, judge.CodeUnreachable},
		{"an unknown verdict", says("maybe", score(99)), judge.CodeUnparsable},
		{"the budget", &fake{hold: make(chan struct{}), answer: judge.Answer{Verdict: "pass"}}, judge.CodeTimeout},
	} {
		j, recs := judge.Run(context.Background(), judge.Stack{configured("a", setup.Decide, 0, c.j)}, request, 50*time.Millisecond)
		want := []judge.Record{{Judge: "a", Use: "decide", Outcome: "none", Error: string(c.code)}}
		if j != nil || !reflect.DeepEqual(recs, want) {
			t.Fatalf("%s: %+v, %s", c.name, j, lines(recs))
		}
	}
	// A judge the budget left unasked says none, timeout, and is never asked.
	slow, after := &fake{hold: make(chan struct{})}, says("pass", nil)
	_, recs := judge.Run(context.Background(), judge.Stack{configured("a", setup.Decide, 0, slow), configured("b", setup.Decide, 0, after)},
		request, 50*time.Millisecond)
	if len(recs) != 2 || recs[1].Error != "timeout" || after.calls.Load() != 0 {
		t.Fatalf("after the budget: %s, b asked %d times", lines(recs), after.calls.Load())
	}
}

// TestAdviseDecidesNothing runs the advise judges of a stack, and only them, each within a budget of
// its own; Run asks none of them. An advise judge still running at its budget is none, timeout.
func TestAdviseDecidesNothing(t *testing.T) {
	t.Parallel()
	a, b := &fake{hold: make(chan struct{})}, says("block", nil)
	d := says("pass", nil)
	s := judge.Stack{configured("a", setup.Advise, 0, a), configured("d", setup.Decide, 0, d), configured("b", setup.Advise, 0, b)}
	recs := judge.Advise(context.Background(), s, request, 50*time.Millisecond)
	want := []judge.Record{{Judge: "a", Use: "advise", Outcome: "none", Error: "timeout"},
		{Judge: "b", Use: "advise", Outcome: "block"}}
	if !reflect.DeepEqual(recs, want) || d.calls.Load() != 0 {
		t.Fatalf("Advise: %s, d asked %d times", lines(recs), d.calls.Load())
	}
	adv := says("pass", nil)
	if j, recs := judge.Run(context.Background(), judge.Stack{configured("a", setup.Advise, 0, adv)}, request, time.Second); j != nil || len(recs) != 0 || adv.calls.Load() != 0 {
		t.Fatalf("Run of an advise judge: %+v, %d records, asked %d times", j, len(recs), adv.calls.Load())
	}
}

// TestLine writes each record's line: canonical, with only the members it has, and output that is
// not valid UTF-8 as standard base64 (docs/adr/0023-judges.md).
func TestLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		r    judge.Record
		want string
	}{
		{judge.Record{Judge: "a", Use: "decide", Outcome: "none", Error: "raw-body"},
			`{"error":"raw-body","judge":"a","outcome":"none","use":"decide"}`},
		{judge.Record{Judge: "a", Use: "advise", Outcome: "pass", Score: score(91), Output: []byte("It is <fine> & safe.")},
			`{"judge":"a","outcome":"pass","output":"It is <fine> & safe.","score":91,"use":"advise"}`},
		{judge.Record{Judge: "a", Use: "decide", Outcome: "none", Abstained: true, Score: score(3), Output: []byte{0xff, 0x00, 0x41}},
			`{"abstained":true,"judge":"a","outcome":"none","outputBase64":"/wBB","score":3,"use":"decide"}`},
		{judge.Record{Judge: "a", Use: "decide", Outcome: "block", Output: []byte("\xef\xbf\xbf")},
			`{"judge":"a","outcome":"block","outputBase64":"77+/","use":"decide"}`},
	} {
		if got := string(judge.Line(c.r)); got != c.want {
			t.Fatalf("Line = %s, want %s", got, c.want)
		}
	}
}

// lines is the records' lines, for a failure's message.
func lines(recs []judge.Record) string {
	var s string
	for _, r := range recs {
		s += string(judge.Line(r)) + " "
	}
	return s
}
