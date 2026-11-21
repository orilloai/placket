package client_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/client"
)

// fixtures is api/testdata/config/, the labels config fixtures, and their verdicts.
const fixtures = "../../api/testdata/config"

func noEnv(string) string { return "" }

// TestTheDecoderGivesEachFixtureItsVerdict reads every fixture of api/testdata/config/ as the client
// reads a labels config: each gets the verdict verdicts.json lists, ok or the code it is refused
// with, the same verdicts core/conformance holds the config's schema to.
func TestTheDecoderGivesEachFixtureItsVerdict(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(fixtures, "verdicts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var verdicts map[string]string
	if err := json.Unmarshal(b, &verdicts); err != nil || len(verdicts) == 0 {
		t.Fatalf("verdicts.json: %v", err)
	}
	for name, want := range verdicts {
		in, err := os.ReadFile(filepath.Join(fixtures, name))
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.DecodeConfig(in, "/config", noEnv)
		got := "ok"
		var ce *client.Error
		if errors.As(err, &ce) {
			got = string(ce.Code)
		} else if err != nil {
			got = err.Error()
		}
		if got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}

// TestTheConfigNamesBothEntries reads configs that fail a check across entries: the error names both
// entries by index, the first pair by i and then j; its text is the package, the code and the
// indexes, and nothing of a path. A config that fails the schema wraps nothing.
func TestTheConfigNamesBothEntries(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"collision-first-pair.json":      "client: mediator-collision 0 3",
		"label-taken-before-socket.json": "client: label-taken 0 1",
		"collision-socket-cleaned.json":  "client: mediator-collision 0 1",
	} {
		in, _ := os.ReadFile(filepath.Join(fixtures, name))
		if _, err := client.DecodeConfig(in, "/config", noEnv); err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %s", name, err, want)
		}
	}
	in, _ := os.ReadFile(filepath.Join(fixtures, "bad-label-upper.json"))
	if _, err := client.DecodeConfig(in, "/config", noEnv); err == nil || err.Error() != "client: bad-config" || errors.Unwrap(err) != nil {
		t.Fatalf("bad-config: %v", err)
	}
}

// TestTheConfigsPaths reads an entry's relative paths under the config directory, cleaned; an entry
// with no socket has its Accord's default, and every entry the command line's state directory.
// Find names a labelled entry by its label only, an unlabelled one by its Accord id, and anything
// else is unknown-mediator. A missing config is an empty list.
func TestTheConfigsPaths(t *testing.T) {
	t.Parallel()
	a, b := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64)
	in := []byte(`{"mediators":[{"accord":"` + a + `","key":"keys/../p.key","founding":"f.json","label":"merge"},
		{"accord":"` + b + `","key":"/abs/d.key","founding":"/abs/f.json","socket":"s/./b.sock"}]}`)
	env := func(n string) string { return map[string]string{"XDG_RUNTIME_DIR": "/run/u"}[n] }
	c, err := client.DecodeConfig(in, "/config/placket", env)
	if err != nil {
		t.Fatal(err)
	}
	want := []client.Mediator{
		{Label: "merge", Accord: a, Key: "/config/placket/p.key", Founding: "/config/placket/f.json", Socket: "/run/u/placket/aaaaaaaaaaaa.sock",
			State: "/config/placket/p.key.state/" + strings.Repeat("a", 64)},
		{Accord: b, Key: "/abs/d.key", Founding: "/abs/f.json", Socket: "/config/placket/s/b.sock", State: "/abs/d.key.state/" + strings.Repeat("b", 64)},
	}
	if !reflect.DeepEqual(c.Mediators, want) {
		t.Fatalf("the entries:\n%+v\nwant\n%+v", c.Mediators, want)
	}
	if m, err := c.Find("merge"); err != nil || m.Accord != a {
		t.Fatalf("merge: %+v, %v", m, err)
	}
	if m, err := c.Find(b); err != nil || m.Key != "/abs/d.key" {
		t.Fatalf("by Accord id: %+v, %v", m, err)
	}
	for _, name := range []string{a, "nope", ""} {
		var ce *client.Error
		if _, err := c.Find(name); !errors.As(err, &ce) || ce.Code != client.CodeUnknownMediator {
			t.Fatalf("Find(%q): %v", name, err)
		}
	}
	dir := t.TempDir()
	cfg, err := client.ReadConfig(func(n string) string { return map[string]string{"XDG_CONFIG_HOME": dir}[n] })
	if err != nil || len(cfg.Mediators) != 0 {
		t.Fatalf("a missing config: %+v, %v", cfg, err)
	}
	if d := client.ConfigDir(func(n string) string { return map[string]string{"HOME": "/home/u"}[n] }); d != "/home/u/.config/placket" {
		t.Fatalf("the config directory under HOME: %s", d)
	}
}

// TestARelativeConfigHomeCountsAsUnset reads the config directory under environments whose
// directories are relative: a relative XDG_CONFIG_HOME is passed over for HOME, and a relative HOME
// with no absolute XDG_CONFIG_HOME is no config directory, as the default journal is, so the config
// is never read from whatever directory the command was started in.
func TestARelativeConfigHomeCountsAsUnset(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"XDG_CONFIG_HOME": "/c", "HOME": "/home/u"}, "/c/placket"},
		{map[string]string{"XDG_CONFIG_HOME": "rel", "HOME": "/home/u"}, "/home/u/.config/placket"},
		{map[string]string{"XDG_CONFIG_HOME": "./c", "HOME": "/home/u"}, "/home/u/.config/placket"},
		{map[string]string{"HOME": "rel"}, ""},
		{map[string]string{"XDG_CONFIG_HOME": "rel", "HOME": "rel"}, ""},
		{map[string]string{}, ""},
	} {
		if got := client.ConfigDir(func(n string) string { return c.env[n] }); got != c.want {
			t.Fatalf("%v: %q, want %q", c.env, got, c.want)
		}
	}
}
