package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/state"
)

// TestReleasedKeptRefusesALineReadTwoWays puts, before the body of a kept part, a second text
// member: one of the same name, then one named in another case. A reader that kept the last of two
// members, or matched names in any case, would read the true body and match the part. released
// refuses the line, so only the parts beside it match (parse-strict).
func TestReleasedKeptRefusesALineReadTwoWays(t *testing.T) {
	t.Parallel()
	k := newKeptAccord(t)
	lines := keptLines(t, k.state("applier"))
	if len(lines) != 3 {
		t.Fatalf("kept %d lines", len(lines))
	}
	for _, name := range []string{"text", "Text"} {
		dir := copyDir(t, k.state("applier"))
		var kept []string
		for i, l := range lines {
			m, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			if i == 1 {
				m = append([]byte(`{"`+name+`":"not the body",`), m[1:]...)
			}
			kept = append(kept, string(m))
		}
		if err := os.WriteFile(filepath.Join(dir, state.FileKept), []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		r := run(t, env(), "released", "--state", dir, "--every")
		keptMatched(t, r, cli.ClaimReleasedKept, k.r1, k.r3)
		keptUnmatched(t, run(t, env(), "released", "--state", dir, "--re", k.q2), cli.LineNoKept)
	}
}
