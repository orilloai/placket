package setup_test

import (
	"reflect"
	"testing"

	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
)

// TestJudgeKinds reads the kinds of every term, of any member, whose resolve lists a judge, in the
// judged vector's Accord: sorted, each once, whatever the use; none for a judge no term lists, nor
// for a nil Accord (setup-contract-1, docs/adr/0023-judges.md). The result is a copy
// (results-are-copies).
func TestJudgeKinds(t *testing.T) {
	t.Parallel()
	v := vectors(t)["judged"]
	a, err := founding.Found(v.inputs(), v.FoundedAt)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"digest", "ledger", "report", "review", "survey"}
	got := a.JudgeKinds("jev")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JudgeKinds(jev) = %v, want %v", got, want)
	}
	got[0] = "changed"
	if a.JudgeKinds("jev")[0] != "digest" {
		t.Fatal("JudgeKinds shares the Accord's state")
	}
	if got := a.JudgeKinds("lint"); len(got) != 0 {
		t.Fatalf("JudgeKinds(lint) = %v, want none", got)
	}
	if got := (*setup.Accord)(nil).JudgeKinds("jev"); len(got) != 0 {
		t.Fatalf("a nil Accord's JudgeKinds = %v", got)
	}
}

// FuzzJudgeKinds reads any judge's kinds in the judged vector's Accord: it never panics, and the kinds
// are sorted and distinct (core-no-panic).
func FuzzJudgeKinds(f *testing.F) {
	v := vectors(f)["judged"]
	a, err := founding.Found(v.inputs(), v.FoundedAt)
	if err != nil {
		f.Fatal(err)
	}
	f.Add("jev")
	f.Add("")
	f.Add("lint")
	f.Fuzz(func(t *testing.T, name string) {
		got := a.JudgeKinds(name)
		for i := 1; i < len(got); i++ {
			if got[i-1] >= got[i] {
				t.Fatalf("JudgeKinds(%q) = %v: not sorted and distinct", name, got)
			}
		}
	})
}
