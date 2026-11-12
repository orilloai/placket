package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadAll files every schema of the repository's spec/schema/ in one set.
func loadAll(t *testing.T) *schemaSet {
	t.Helper()
	set := newSchemaSet()
	files, err := filepath.Glob(filepath.Join(repoSpec, "schema", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no schema in %s/schema: %v", repoSpec, err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := set.add(f, b); err != nil {
			t.Fatalf("%v", err)
		}
	}
	return set
}

// TestSchemaExamples holds the runner's validator to every schema's examples, the same examples
// schemas-hold checks with a validator of its own: each accepted example validates and each refused
// one is refused. A schema keyword the validator does not read fails the compile, so no example
// passes for a keyword that was skipped.
func TestSchemaExamples(t *testing.T) {
	t.Parallel()
	set := loadAll(t)
	dirs, err := filepath.Glob(filepath.Join(repoSpec, "schema", "testdata", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	total := 0
	for _, d := range dirs {
		name := filepath.Base(d)
		root, err := set.compile("https://placket.dev/schema/0.1/" + name + ".json#")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, side := range []string{"accepted", "refused"} {
			examples, _ := filepath.Glob(filepath.Join(d, side, "*.json"))
			if len(examples) == 0 {
				t.Fatalf("%s: no %s example", name, side)
			}
			for _, e := range examples {
				total++
				b, err := os.ReadFile(e)
				if err != nil {
					t.Fatal(err)
				}
				v, err := read(b)
				var bad *schemaError
				if err == nil {
					bad = root.validate(v, "/")
				}
				switch {
				case side == "accepted" && err != nil:
					t.Errorf("%s: cannot be read: %v", e, err)
				case side == "accepted" && bad != nil:
					t.Errorf("%s: refused: %v", e, bad)
				case side == "refused" && err == nil && bad == nil:
					t.Errorf("%s: accepted", e)
				}
			}
		}
	}
	if total < 100 {
		t.Fatalf("only %d examples read", total)
	}
}

// TestSchemaRules checks each keyword the validator reads, and the message of each refusal.
func TestSchemaRules(t *testing.T) {
	t.Parallel()
	const id = "https://example.test/s/"
	for _, c := range []struct{ schema, value, want string }{
		{`true`, `{"a":1}`, ``},
		{`false`, `1`, `/: not allowed here (a.json#/properties/x)`},
		{`{"type":"string"}`, `1`, `/: is an integer, not string`},
		{`{"type":["string","null"]}`, `null`, ``},
		{`{"type":"integer"}`, `1.0`, ``},
		{`{"type":"integer"}`, `1e400`, ``},
		{`{"type":"integer"}`, `1.5`, `/: is a number, not integer`},
		{`{"type":"object"}`, `[]`, `/: is an array, not object`},
		{`{"type":"boolean"}`, `{}`, `/: is an object, not boolean`},
		{`{"type":"null"}`, `"x"`, `/: is a string, not null`},
		{`{"const":"0.1"}`, `"0.2"`, `/: is not "0.1"`},
		{`{"const":{"b":[1,2],"a":null}}`, `{"a":null,"b":[1.0,2]}`, ``},
		{`{"const":{"a":1}}`, `{"a":1,"b":1}`, `/: is not {"a":1}`},
		{`{"enum":["a","b"]}`, `"c"`, `/: is not one of ["a","b"]`},
		{`{"enum":[1]}`, `10e-1`, ``},
		{`{"pattern":"^[a-z]+$"}`, `"aB"`, `/: does not match ^[a-z]+$`},
		{`{"pattern":"^[a-z]+$"}`, `1`, ``},
		{`{"minLength":2}`, `"é"`, `/: is shorter than 2 characters`},
		{`{"maxLength":1}`, `"é"`, ``},
		{`{"maxLength":1}`, `"ab"`, `/: is longer than 1 characters`},
		{`{"minimum":0}`, `-1e-500`, `/: is less than the minimum (a.json#/properties/x)`},
		{`{"minimum":0}`, `0.0`, ``},
		{`{"maximum":9007199254740991}`, `9007199254740992`, `/: is more than the maximum (a.json#/properties/x)`},
		{`{"maximum":9007199254740991}`, `9007199254740991.0`, ``},
		{`{"maximum":1}`, `1e99999999999999999999`, `/: is more than the maximum (a.json#/properties/x)`},
		{`{"required":["a"]}`, `{"b":1}`, `/: has no member "a"`},
		{`{"properties":{"a":{"type":"string"}}}`, `{"a":1}`, `/a: is an integer, not string`},
		{`{"properties":{"a/b":{"type":"string"}}}`, `{"a/b":1}`, `/a~1b: is an integer, not string`},
		{`{"properties":{"a":true},"additionalProperties":false}`, `{"a":1,"A":1}`, `/: has a member "A" the format does not list`},
		{`{"additionalProperties":{"type":"string"}}`, `{"a":"x","b":2}`, `/b: is an integer, not string`},
		{`{"propertyNames":{"enum":["a"]}}`, `{"a":1,"b":1}`, `/: has a member "b" whose name is refused: it is not one of ["a"]`},
		{`{"minProperties":1}`, `{}`, `/: has fewer than 1 members`},
		{`{"maxProperties":1}`, `{"a":1,"b":2}`, `/: has more than 1 members`},
		{`{"items":{"type":"string"}}`, `["a",1]`, `/1: is an integer, not string`},
		{`{"minItems":1}`, `[]`, `/: has fewer than 1 items`},
		{`{"maxItems":1}`, `[1,2]`, `/: has more than 1 items`},
		{`{"uniqueItems":true}`, `[1,{"a":1},1.0]`, `/: repeats an item: 0 and 2 are equal`},
		{`{"uniqueItems":true}`, `[{"a":1},{"a":2}]`, ``},
		{`{"allOf":[{"type":"string"},{"minLength":3}]}`, `"ab"`, `/: is shorter than 3 characters`},
		{`{"anyOf":[{"type":"string"},{"type":"null"}]}`, `1`, `/: holds none of the schemas of a.json#/properties/x/anyOf`},
		{`{"oneOf":[{"type":"string"},{"minLength":1}]}`, `"a"`, `/: holds 2 of the 2 schemas of a.json#/properties/x/oneOf, not exactly one`},
		{`{"oneOf":[{"type":"string"},{"type":"null"}]}`, `1`, `/: holds 0 of the 2 schemas of a.json#/properties/x/oneOf, not exactly one`},
		{`{"oneOf":[{"required":["a"]},{"properties":{"b":{"type":"string"}},"required":["b"]}]}`, `{"b":1}`,
			`/b: is an integer, not string`},
		{`{"not":{"const":"ok"}}`, `"ok"`, `/: holds the schema of a.json#/properties/x/not`},
		{`{"if":{"const":1},"then":{"type":"integer","maximum":0}}`, `1`, `/: is more than the maximum (a.json#/properties/x/then)`},
		{`{"if":{"const":1},"then":false,"else":{"type":"string"}}`, `2`, `/: is an integer, not string`},
		{`{"if":{"const":1},"else":false}`, `1`, ``},
		{`{"$ref":"#/$defs/s"}`, `1`, `/: is an integer, not string`},
		{`{"$ref":"b.json#/$defs/n"}`, `"x"`, `/: is a string, not number`},
		{`{"$ref":"b.json"}`, `[]`, `/: is an array, not object`},
		{`{"$ref":"` + id + `b.json#/$defs/n"}`, `1`, ``},
		{`{"title":"t","description":"d","$comment":"c","examples":[1],"default":1}`, `"x"`, ``},
	} {
		set := newSchemaSet()
		a := `{"$schema":"` + draft + `","$id":"` + id + `a.json","$defs":{"s":{"type":"string"}},"properties":{"x":` + c.schema + `}}`
		b := `{"$schema":"` + draft + `","$id":"` + id + `b.json","type":"object","$defs":{"n":{"type":"number"}}}`
		if err := set.add("a.json", []byte(a)); err != nil {
			t.Fatal(err)
		}
		if err := set.add("b.json", []byte(b)); err != nil {
			t.Fatal(err)
		}
		n, err := set.compile(id + "a.json#/properties/x")
		if err == nil {
			err = set.link()
		}
		if err != nil {
			t.Errorf("%s: does not compile: %v", c.schema, err)
			continue
		}
		v, err := read([]byte(c.value))
		if err != nil {
			t.Fatalf("%s: %v", c.value, err)
		}
		got := ""
		if e := n.validate(v, "/"); e != nil {
			got = e.Error()
		}
		if got != c.want {
			t.Errorf("%s against %s:\n  got  %q\n  want %q", c.value, c.schema, got, c.want)
		}
	}
}

// TestSchemaCompileRefusals checks that a schema the validator cannot read in full is refused, never
// read in part.
func TestSchemaCompileRefusals(t *testing.T) {
	t.Parallel()
	const id = "https://example.test/s/"
	for _, c := range []struct{ schema, want string }{
		{`{"format":"date-time"}`, `a.json#/properties/x: the keyword "format" is not one this runner reads`},
		{`{"unevaluatedProperties":false}`, `a.json#/properties/x: the keyword "unevaluatedProperties" is not one this runner reads`},
		{`{"type":"text"}`, `a.json#/properties/x: type "text" is not a JSON Schema type`},
		{`{"pattern":"(?<n>x)\\k<n>"}`, `a.json#/properties/x: pattern does not compile: error parsing regexp: invalid escape sequence: ` + "`\\k`"},
		{`{"minLength":-1}`, `a.json#/properties/x: minLength is not a whole number written in digits`},
		{`{"allOf":[]}`, `a.json#/properties/x: allOf is not a list of schemas`},
		{`{"items":1}`, `a.json#/properties/x/items: a schema that is neither an object nor a boolean`},
		{`{"$ref":"c.json"}`, `no schema has the $id https://example.test/s/c.json`},
		{`{"$ref":"#/$defs/none"}`, `https://example.test/s/a.json#/$defs/none: the pointer names nothing`},
	} {
		set := newSchemaSet()
		a := `{"$schema":"` + draft + `","$id":"` + id + `a.json","properties":{"x":` + c.schema + `}}`
		if err := set.add("a.json", []byte(a)); err != nil {
			t.Fatal(err)
		}
		_, err := set.compile(id + "a.json#/properties/x")
		if err == nil {
			err = set.link()
		}
		if err == nil || err.Error() != c.want {
			t.Errorf("%s:\n  got  %v\n  want %q", c.schema, err, c.want)
		}
	}
	for _, c := range []struct{ doc, want string }{
		{`[]`, `a.json: not a JSON object`},
		{`{"$id":"` + id + `a.json"}`, `a.json: its $schema is not ` + draft},
		{`{"$schema":"` + draft + `"}`, `a.json: no $id naming the document`},
		{`{"$schema":"` + draft + `","$id":"` + id + `a.json#x"}`, `a.json: no $id naming the document`},
		{`{"$schema":"` + draft + `","$id":"` + id + `b.json"}`, `a.json: a second schema with the $id ` + id + `b.json`},
	} {
		set := newSchemaSet()
		if err := set.add("b.json", []byte(`{"$schema":"`+draft+`","$id":"`+id+`b.json"}`)); err != nil {
			t.Fatal(err)
		}
		if err := set.add("a.json", []byte(c.doc)); err == nil || err.Error() != c.want {
			t.Errorf("%s:\n  got  %v\n  want %q", c.doc, err, c.want)
		}
	}
}

func TestDecimal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		a, b string
		cmp  int
	}{
		{"0", "-0", 0},
		{"0", "0e99999999999999999999", 0},
		{"1", "1.0", 0},
		{"1", "10e-1", 0},
		{"100", "1e2", 0},
		{"1", "2", -1},
		{"-1", "1", -1},
		{"-2", "-1", -1},
		{"0.1", "0.09", 1},
		{"9007199254740991", "9007199254740992", -1},
		{"1e400", "9007199254740991", 1},
		{"-1e400", "-9007199254740991", -1},
		{"1e-400", "0", 1},
		{"123.456", "123.4560", 0},
		{"1E+2", "100", 0},
	} {
		got := parseDecimal(number(c.a)).cmp(parseDecimal(number(c.b)))
		if got != c.cmp {
			t.Errorf("cmp(%s, %s) = %d, want %d", c.a, c.b, got, c.cmp)
		}
		if back := parseDecimal(number(c.b)).cmp(parseDecimal(number(c.a))); back != -c.cmp {
			t.Errorf("cmp(%s, %s) = %d, want %d", c.b, c.a, back, -c.cmp)
		}
	}
	for s, want := range map[string]bool{"1": true, "1.0": true, "1.5": false, "15e-1": false, "1e400": true, "0.0": true, "-3": true, "1e-400": false} {
		if got := parseDecimal(number(s)).integral(); got != want {
			t.Errorf("integral(%s) = %v, want %v", s, got, want)
		}
	}
}

// FuzzValidate checks that no value, however it is built, makes the validator stop.
func FuzzValidate(f *testing.F) {
	f.Add([]byte(`{"vector":"canonical/x","spec":"0.1","tests":"t","operation":"canonical","inputHex":"","expect":{"code":"invalid-json","offset":0}}`))
	f.Add([]byte(`{"operation":"scenario","steps":[{"op":"give"},{"op":"resolve","action":"changed","content":{"answer":""}}],"final":{"pending":[1,1]}}`))
	f.Add([]byte(`[1e99999999999999999999,-0.0,{"":null}]`))
	set := newSchemaSet()
	files, _ := filepath.Glob(filepath.Join(repoSpec, "schema", "*.json"))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		if err := set.add(p, b); err != nil {
			f.Fatal(err)
		}
	}
	root, err := set.compile("https://placket.dev/schema/0.1/vector.json#")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := read(b)
		if err != nil {
			return
		}
		if e := root.validate(v, "/"); e != nil && !strings.HasPrefix(e.at, "/") {
			t.Fatalf("an error not at a pointer: %q", e.Error())
		}
	})
}
