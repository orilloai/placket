package eval_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/orilloai/placket/adapters/judge/eval"
)

// FuzzReadCases reads any bytes as a case file, seeded from the lines of evals/judge/cases.jsonl and
// the refusal tests' files: it never panics, and every case it reads keeps the file's rules
// (fuzz-parsers).
func FuzzReadCases(f *testing.F) {
	b, err := os.ReadFile("../../../evals/judge/cases.jsonl")
	if err != nil {
		f.Fatal(err)
	}
	for _, l := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		f.Add(l)
	}
	for _, c := range caseLines {
		f.Add([]byte(c.file))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		cases, err := eval.ReadCases(b)
		if err != nil {
			return
		}
		for _, c := range cases {
			if c.ID == "" || (c.Expect != "pass" && c.Expect != "block" && c.Expect != "abstain") || (c.Injection && c.Expect == "pass") {
				t.Fatalf("read %+v", c)
			}
		}
	})
}
