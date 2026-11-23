package export

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestTheDocsSayWhatADeclineShows reads, from the tree, the log's line in decision record 0011 and
// the column list of this package's comment: each says that a decline's signer is not compared, and
// that the mediator took a decline only from an answerer (maybe-6). The record no longer says that a
// decline's call verifies under the row's answerer.
func TestTheDocsSayWhatADeclineShows(t *testing.T) {
	t.Parallel()
	record, err := os.ReadFile("../../docs/adr/0011-the-door-and-the-journal.md")
	if err != nil {
		t.Fatal(err)
	}
	var logLine string
	for _, l := range strings.Split(string(record), "\n") {
		if strings.HasPrefix(l, "- **The log**") {
			logLine = l
		}
	}
	f, err := parser.ParseFile(token.NewFileSet(), "export.go", nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil || f.Doc == nil {
		t.Fatalf("export.go's package comment: %v", err)
	}
	// The column's description runs from its name to the next column's.
	doc := f.Doc.Text()
	_, column, _ := strings.Cut(doc, "\tanswer_signed ")
	column, _, _ = strings.Cut(column, "\tanswerer ")
	for what, text := range map[string]string{"record 0011's log line": logLine, "the answer_signed column": column} {
		for _, phrase := range []string{"a decline's signer is not compared", "(maybe-6)"} {
			if !strings.Contains(strings.Join(strings.Fields(text), " "), phrase) {
				t.Errorf("%s does not say %q: %q", what, phrase, text)
			}
		}
	}
	if strings.Contains(logLine, "verifies under the row's answerer") {
		t.Errorf("record 0011's log line still says a call verifies under the row's answerer: %q", logLine)
	}
}
