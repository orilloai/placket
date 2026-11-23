package local_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/receipts"
)

// advised waits for the window's advised event and returns it.
func advised(t *testing.T, r *running) local.Event {
	t.Helper()
	for i := 0; i < 1000; i++ {
		r.mu.Lock()
		for _, e := range *r.events {
			if e.Kind == local.EventAdvised {
				r.mu.Unlock()
				return e
			}
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no advised event")
	return local.Event{}
}

// TestAdviseNeverDecidesNorDelays stacks advise a, held, and decide b, which passes: the give's reply,
// pass, ok, by the judge b, comes while a is held; released, a's line is appended to the same judged
// file, use advise, and the window emits what it said (docs/adr/0009-who-decided.md).
func TestAdviseNeverDecidesNorDelays(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a", "b"}, uses("a", "advise", "b", "decide"), false)
	j.fakes["a"].hold = make(chan struct{})
	j.fakes["a"].verdict = "block"
	j.fakes["b"].verdict = "pass"
	r := start(t, j.window(t))
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "pass" || rep.Outcome.Reason != "ok" ||
		byOf(t, rep.Outcome.Receipt) != (receipts.By{Decider: receipts.ByJudge, Judge: "b"}) {
		t.Fatalf("the report: %+v", rep.Outcome)
	}
	if got := j.judgedLines(t, rep.Outcome.ReceiptID); len(got) != 1 {
		t.Fatalf("judged lines before a returns: %v", got)
	}
	close(j.fakes["a"].hold)
	e := advised(t, r)
	if want := []local.Advice{{Judge: "a", Outcome: "block", Receipt: rep.Outcome.ReceiptID}}; !reflect.DeepEqual(e.Advice, want) {
		t.Fatalf("the advice: %+v", e.Advice)
	}
	want := []string{`{"judge":"b","outcome":"pass","output":"` + plantedOutput + `","use":"decide"}`,
		`{"judge":"a","outcome":"block","output":"` + plantedOutput + `","use":"advise"}`}
	if got := j.judgedLines(t, rep.Outcome.ReceiptID); !reflect.DeepEqual(got, want) {
		t.Fatalf("judged lines %v", got)
	}
}

// receiptsAt is a judge that, when asked, notes how many receipts the journal holds.
type receiptsAt struct {
	*fakeJudge
	dir  string
	seen chan int
}

func (r *receiptsAt) Judge(ctx context.Context, req judge.Request) (judge.Answer, error) {
	b, _ := os.ReadFile(filepath.Join(r.dir, journal.FileReceipts))
	r.seen <- bytes.Count(b, []byte("\n"))
	return r.fakeJudge.Judge(ctx, req)
}

// TestAdviseOnlyAsks gives a report whose rule lists only advise a, which blocks: the give is asked,
// and a is asked only once the ask's receipt is written. With a held until its budget of 50 ms ends,
// its line is none, timeout.
func TestAdviseOnlyAsks(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "advise"), false)
	j.fakes["a"].verdict = "block"
	at := &receiptsAt{fakeJudge: j.fakes["a"], dir: j.journal, seen: make(chan int, 1)}
	cfg := j.window(t)
	cfg.NewJudge = func(judge.Config) judge.Judge { return at }
	r := start(t, cfg)
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" || rep.Outcome.Reason != "maybe" {
		t.Fatalf("the report: %+v", rep.Outcome)
	}
	if n := <-at.seen; n != 1 {
		t.Fatalf("a was asked with %d receipts journaled", n)
	}
	advised(t, r)

	slow := newJudged(t, []string{"a"}, uses("a", "advise"), false)
	slow.fakes["a"].hold = make(chan struct{})
	cfg = slow.window(t)
	cfg.Budget = 50 * time.Millisecond
	r = start(t, cfg)
	rep = wait(t, slow.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	advised(t, r)
	if got := slow.judgedLines(t, rep.Outcome.ReceiptID); !reflect.DeepEqual(got, []string{`{"error":"timeout","judge":"a","outcome":"none","use":"advise"}`}) {
		t.Fatalf("judged lines %v", got)
	}
}

// TestARawBodyIsAdvisedNothing gives a raw report under advise a and decide b: neither is asked, and
// both lines are none, raw-body, each with its use.
func TestARawBodyIsAdvisedNothing(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a", "b"}, uses("a", "advise", "b", "decide"), false)
	start(t, j.window(t))
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "raw", base64.StdEncoding.EncodeToString([]byte{0xff})))
	time.Sleep(50 * time.Millisecond)
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" || j.fakes["a"].calls.Load()+j.fakes["b"].calls.Load() != 0 {
		t.Fatalf("a raw report: %+v", rep.Outcome)
	}
	want := []string{`{"error":"raw-body","judge":"a","outcome":"none","use":"advise"}`,
		`{"error":"raw-body","judge":"b","outcome":"none","use":"decide"}`}
	if got := j.judgedLines(t, rep.Outcome.ReceiptID); !reflect.DeepEqual(got, want) {
		t.Fatalf("judged lines %v", got)
	}
}

// TestStoppingStopsTheAdvice stops the window while an advise judge is held: Run returns once it has
// returned, and no line is written after.
func TestStoppingStopsTheAdvice(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "advise"), false)
	j.fakes["a"].hold = make(chan struct{})
	r := start(t, j.window(t))
	wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	for j.fakes["a"].calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	r.stop(t)
	if j.fakes["a"].returns.Load() != 1 {
		t.Fatal("Run returned before the advise judge did")
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(j.journal, journal.DirJudged)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("judged/ after the stop: %v", err)
	}
}

// TestAnAdviseAppendThatFails fails the judged/ append of an advise run: the give's reply was sent,
// Run returns the journal's error, and receipts.jsonl holds the receipt (journal-synced).
func TestAnAdviseAppendThatFails(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "advise"), false)
	j.disk = failingJudged{}
	r := start(t, j.window(t))
	if rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report.")); rep.Outcome == nil {
		t.Fatalf("the report: %+v", rep)
	}
	select {
	case err := <-r.done:
		var je *journal.Error
		if !errors.As(err, &je) || !strings.HasPrefix(je.File, journal.DirJudged+"/") {
			t.Fatalf("the window ended with %v", err)
		}
		r.cancel = nil
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not end")
	}
	if lines := bytes.Count(j.receipts(t), []byte("\n")); lines != 1 {
		t.Fatalf("receipts.jsonl holds %d lines", lines)
	}
}

// TestNothingPrintsTheAdvice gives a report whose advise judge says block with a planted output, and
// whose decide judge fails: neither the window's error writer nor any event holds the output
// (secrets-never-print).
func TestNothingPrintsTheAdvice(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a", "b"}, uses("a", "advise", "b", "decide"), false)
	j.fakes["a"].verdict = "block"
	cfg := j.window(t)
	failed := &erring{}
	cfg.NewJudge = func(c judge.Config) judge.Judge {
		if c.Name == "b" {
			return failed
		}
		return j.fakes[c.Name]
	}
	r := start(t, cfg)
	wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	advised(t, r)
	r.stop(t)
	r.mu.Lock()
	defer r.mu.Unlock()
	printed := r.stderr.String()
	for _, e := range *r.events {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
			printed += fmt.Sprintf(verb, e)
		}
	}
	if strings.Contains(printed, "planted") || strings.Contains(printed, hex.EncodeToString([]byte("planted"))) {
		t.Fatal("an event or the error writer holds the model's output")
	}
}

// erring is a judge whose every request fails, with an error that quotes a planted output.
type erring struct{}

func (erring) Judge(context.Context, judge.Request) (judge.Answer, error) {
	return judge.Answer{}, errors.New(plantedOutput)
}
