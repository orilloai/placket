package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
)

// repoAPI is the repository's api/, two levels up from this package, read by path as spec/ and
// vectors/ are.
const repoAPI = "../../api"

const apiBase = "https://placket.tech/api/0.1/"

// apiSchemas files every schema under api/wire/, api/request/ and api/config/ in one set, and
// returns it with the $id of each.
func apiSchemas(t *testing.T) (*schemaSet, []string) {
	t.Helper()
	set := newSchemaSet()
	var ids []string
	for _, dir := range []string{"wire", "request", "config"} {
		files, err := filepath.Glob(filepath.Join(repoAPI, dir, "*.json"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no schema in api/%s: %v", dir, err)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := set.add(f, b); err != nil {
				t.Fatalf("%v", err)
			}
			id := apiBase + dir + "/" + filepath.Base(f)
			ids = append(ids, id)
		}
	}
	return set, ids
}

// validateFile validates a JSON file, or one of its members when member is not empty, against the
// schema of the $id.
func validateFile(t *testing.T, set *schemaSet, id, path, member string) *schemaError {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if member != "" {
		var o map[string]json.RawMessage
		if err := json.Unmarshal(b, &o); err != nil || o[member] == nil {
			t.Fatalf("%s: no %s: %v", path, member, err)
		}
		b = o[member]
	}
	v, err := read(b)
	if err != nil {
		t.Fatalf("%s: cannot be read: %v", path, err)
	}
	root, err := set.compile(id + "#")
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return root.validate(v, "/")
}

// TestAPISchemas holds the mediator API's schemas under api/ to what they describe: every schema
// compiles, each under its own $id; each worked call of api/testdata/worked/ validates against its
// op's call schema, and its bytes are its call's canonical form; each reply recorded in
// api/testdata/replies/, <op>.json or <op>.<what>.json, validates against its op's reply schema; and
// of the config fixtures of api/testdata/config/, the schema refuses exactly those
// api/testdata/config/verdicts.json lists as bad-config, and every fixture has a verdict.
func TestAPISchemas(t *testing.T) {
	t.Parallel()
	set, ids := apiSchemas(t)
	for _, id := range ids {
		if _, err := set.compile(id + "#"); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	ops := []string{"give", "get", "questions", "resolve", "result"}
	for _, op := range ops {
		path := filepath.Join(repoAPI, "testdata", "worked", op+".json")
		if bad := validateFile(t, set, apiBase+"wire/"+op+".call.json", path, "call"); bad != nil {
			t.Errorf("%s: refused: %v", path, bad)
		}
		var w struct {
			Bytes string          `json:"bytes"`
			Call  json.RawMessage `json:"call"`
		}
		b, _ := os.ReadFile(path)
		if err := json.Unmarshal(b, &w); err != nil {
			t.Fatal(err)
		}
		if c, err := canonical.Form(w.Call); err != nil || string(c) != w.Bytes {
			t.Errorf("%s: its bytes are not its call's canonical form: %v", path, err)
		}
	}
	replies, err := filepath.Glob(filepath.Join(repoAPI, "testdata", "replies", "*.json"))
	if err != nil || len(replies) < len(ops) {
		t.Fatalf("api/testdata/replies holds %d replies: %v", len(replies), err)
	}
	seen := map[string]bool{}
	for _, r := range replies {
		op, _, _ := strings.Cut(filepath.Base(r), ".")
		seen[op] = true
		if bad := validateFile(t, set, apiBase+"wire/"+op+".reply.json", r, ""); bad != nil {
			t.Errorf("%s: refused: %v", r, bad)
		}
	}
	for _, op := range ops {
		if !seen[op] {
			t.Errorf("no reply of %s is recorded", op)
		}
	}
	dir := filepath.Join(repoAPI, "testdata", "config")
	vb, err := os.ReadFile(filepath.Join(dir, "verdicts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var verdicts map[string]string
	if err := json.Unmarshal(vb, &verdicts); err != nil {
		t.Fatal(err)
	}
	fixtures, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var names []string
	for _, f := range fixtures {
		if n := filepath.Base(f); n != "verdicts.json" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) != len(verdicts) || len(names) == 0 {
		t.Fatalf("%d config fixtures, %d verdicts", len(names), len(verdicts))
	}
	for _, n := range names {
		v, ok := verdicts[n]
		switch v {
		case "ok", "bad-config", "label-taken", "mediator-collision":
		default:
			t.Errorf("%s: the verdict %q (listed %v)", n, v, ok)
			continue
		}
		bad := validateFile(t, set, apiBase+"config/mediators.json", filepath.Join(dir, n), "")
		if (bad != nil) != (v == "bad-config") {
			t.Errorf("%s: listed %s, and the schema says %v", n, v, bad)
		}
	}
}
