package kit

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestGeneratorsImportNoCore reads the imports of every Go file under vectors/: the kit imports the
// standard library alone, and a generator the standard library and the kit. So no vector's bytes
// come from the core, through the kit or any other path (vectors-independent).
func TestGeneratorsImportNoCore(t *testing.T) {
	t.Parallel()
	const kitPath = "github.com/orilloai/placket/vectors/signatures/kit"
	files := 0
	err := filepath.WalkDir("../..", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		files++
		inKit := strings.HasSuffix(filepath.ToSlash(filepath.Dir(p)), "/signatures/kit")
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			std := !strings.Contains(strings.Split(path, "/")[0], ".")
			if !std && (inKit || path != kitPath) {
				t.Errorf("%s imports %s", p, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 7 {
		t.Fatalf("only %d Go files under vectors/", files)
	}
}

// TestCanonSortsByUTF16AndEscapesOnlyWhatItMust checks the kit's writer of canonical bytes against
// canonical-1's scenarios, written out by hand.
func TestCanonSortsByUTF16AndEscapesOnlyWhatItMust(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		v    any
		want string
	}{
		{Obj{"b": 1, "a": []any{true}}, `{"a":[true],"b":1}`},
		{Obj{"ﬁ": 1, "\U0001F600": 2}, "{\"\U0001F600\":2,\"ﬁ\":1}"},
		{"\"\\\x01\té\x7f", "\"\\\"\\\\\\u0001\\té\x7f\""},
		{[]any{nil, false, int64(-3), Raw(`{"x":1}`)}, `[null,false,-3,{"x":1}]`},
	} {
		if got := Canon(c.v); !bytes.Equal(got, []byte(c.want)) {
			t.Fatalf("Canon(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}
