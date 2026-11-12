package export

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/receipts"
)

// TestClosingReadsTheReceiptAlone gives each closing receipt its word: accepted and changed by the
// kind of a pass, the four reasons of a block that closes without an answer, and blocked for an
// answer or a version that did not cross.
func TestClosingReadsTheReceiptAlone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		verdict, reason, kind, want string
	}{
		{"pass", "ok", receipts.NameAnswer, "accepted"},
		{"pass", "ok", receipts.NameChanged, "changed"},
		{"block", "declined", "proposal", "declined"},
		{"block", "cancelled", "proposal", "cancelled"},
		{"block", "timeout", "proposal", "timeout"},
		{"block", "undeliverable", "proposal", "undeliverable"},
		{"block", "no-match", receipts.NameAnswer, "blocked"},
		{"block", "maybe", receipts.NameChanged, "blocked"},
	}
	for _, c := range cases {
		got := Closing(receipts.Predicate{Kind: receipts.KindResolution, Verdict: c.verdict, Reason: c.reason}, receipts.Subject{Name: c.kind})
		if got != c.want {
			t.Fatalf("%v: got %q", c, got)
		}
	}
}

// TestCSVQuotesAndDisarms writes rows whose cells hold a comma, a quote, a line feed and the four
// characters a spreadsheet starts a formula with: the cells are quoted per RFC 4180 and each formula
// starts with ' (LOG-27).
func TestCSVQuotesAndDisarms(t *testing.T) {
	t.Parallel()
	l := Log{Rows: []Row{{Proposal: "p", Topic: "a, \"b\"\nc", Comment: "=1+1", Text: "+x", ChangedText: "-y", Answerer: "@z", Receipts: []string{"r1", "r2"}}}}
	var b bytes.Buffer
	if err := l.CSV(&b, true, false); err != nil {
		t.Fatal(err)
	}
	want := "proposal,topic,kind,asked_at,answered_at,answer,released_at,comment,answer_signed,answerer,receipts,text,changed_text\n" +
		"p,\"a, \"\"b\"\"\nc\",,,,,,'=1+1,,'@z,r1 r2,'+x,'-y\n"
	if b.String() != want {
		t.Fatalf("got %q", b.String())
	}
	b.Reset()
	if err := l.CSV(&b, false, true); err != nil {
		t.Fatal(err)
	}
	if want := "proposal,topic,kind,asked_at,answered_at,answer,released_at,comment,answer_signed,answerer,receipts\n" +
		"p,\"a, \"\"b\"\"\nc\",,,,,,,,'@z,r1 r2\n"; b.String() != want {
		t.Fatalf("without words: got %q", b.String())
	}
}

// TestJSONCarriesTheClaim writes the log as JSON: one object with the Accord, the mediator, the claim
// and the scope line as given, and the rows; words add text and changed_text.
func TestJSONCarriesTheClaim(t *testing.T) {
	t.Parallel()
	l := Log{Accord: "a", Mediator: "m", Rows: []Row{{Proposal: "p", Answer: "open", Receipts: []string{"p"}, Text: "t"}}}
	want := `{"accord":"a","claim":"c","mediator":"m","notClaimed":"n","rows":[{"answer":"open","answer_signed":"","answered_at":"","answerer":"","asked_at":"","comment":"","kind":"","proposal":"p","receipts":["p"],"released_at":"","topic":""}]}`
	if got := string(l.JSON("c", "n", false, false)); got != want {
		t.Fatalf("got %s", got)
	}
	if got := string(l.JSON("c", "n", true, false)); !strings.Contains(got, `"changed_text":"","comment":""`) || !strings.Contains(got, `"text":"t"`) {
		t.Fatalf("with words: got %s", got)
	}
}

// TestBuildRefusesWhatDoesNotVerify builds no log from receipts that do not verify, and an empty
// log from none.
func TestBuildRefusesWhatDoesNotVerify(t *testing.T) {
	t.Parallel()
	m := "ed25519:" + strings.Repeat("1", 64)
	_, err := Build(m, [][]byte{[]byte(`{}`)}, nil, func(string) {})
	if e, ok := err.(*Error); !ok || e.Code != CodeVerify || e.Index != 0 || e.Core != "payload-type" {
		t.Fatalf("got %v", err)
	}
	l, err := Build(m, nil, nil, func(string) {})
	if err != nil || len(l.Rows) != 0 || l.Accord != "" {
		t.Fatalf("got %+v, %v", l, err)
	}
}

// FuzzReadKept reads any bytes as a kept line, both as a given part and as a fetched one. Neither
// panics, and a part read from a fetched line names its kind and salt.
func FuzzReadKept(f *testing.F) {
	msg := base64.StdEncoding.EncodeToString([]byte(`{"metadata":{"https://placket.dev/crossing/0.1":{"topic":"t","re":"r"}},"parts":[{"text":"x","metadata":{"https://placket.dev/crossing/0.1":{"kind":"proposal"}}}]}`))
	f.Add([]byte(`{"message":"` + msg + `","receipt":{}}`))
	f.Add([]byte(`{"kind":"answer","messageId":"m","receipt":{},"receiptId":"r","salt":"00000000000000000000000000000000","text":"yes","answerCall":{"call":"","sig":""}}`))
	f.Add([]byte(`{"kind":"k","messageId":"m","raw":"AAE=","receipt":{},"salt":"00000000000000000000000000000000"}`))
	f.Add([]byte(`{"data":{"a":[1]},"kind":"k","messageId":"m","receipt":{},"salt":"00000000000000000000000000000000"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = readGiven(b)
		if p, _, ok := readFetched(b); ok && (p.kind == "" && !bytes.Contains(b, []byte(`"kind"`)) || len(p.salt) < 16) {
			t.Fatalf("read %q", b)
		}
		_, _, _ = readKept(b, nil, map[string]int{})
	})
}
