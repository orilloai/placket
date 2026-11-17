package cli_test

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/core/composition"
)

// The build info of a build at a commit, and of one with no version control information.
var (
	testRevision = "abcdef1234567890abcdef1234567890abcdef12"
	testTime     = "2026-12-01T00:00:00Z"
)

func buildInfo(version string, vcs bool) *debug.BuildInfo {
	bi := &debug.BuildInfo{GoVersion: "go1.27.4", Main: debug.Module{Path: "github.com/orilloai/placket", Version: version}}
	bi.Settings = []debug.BuildSetting{{Key: "CGO_ENABLED", Value: "0"}, {Key: "-trimpath", Value: "true"}}
	if vcs {
		bi.Settings = append(bi.Settings,
			debug.BuildSetting{Key: "vcs", Value: "git"},
			debug.BuildSetting{Key: "vcs.revision", Value: testRevision},
			debug.BuildSetting{Key: "vcs.time", Value: testTime},
			debug.BuildSetting{Key: "vcs.modified", Value: "false"})
	}
	return bi
}

// moduleLine is the line that names this build's decision module, as card --default prints it.
func moduleLine() string {
	return "module " + composition.KernelVersion + " " + composition.KernelHash()
}

// moduleJSON is the module member, as verify's object holds it.
func moduleJSON() string {
	return `{"hash":"` + composition.KernelHash() + `","version":"` + composition.KernelVersion + `"}`
}

// TestVersionLines names the build: its version, its commit and commit time when the build has
// them, its Go and its decision module (spec/protocol.md, section 11, commands-1).
func TestVersionLines(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		bi   *debug.BuildInfo
		want []string
	}{
		{"a tag", buildInfo("v0.0.1-test", true), []string{
			"version v0.0.1-test", "commit " + testRevision, "commitTime " + testTime, "go go1.27.4", moduleLine()}},
		{"a dirty pseudo-version", buildInfo("v0.1.1-0.20261201000000-abcdef123456+dirty", true), []string{
			"version v0.1.1-0.20261201000000-abcdef123456+dirty", "commit " + testRevision, "commitTime " + testTime, "go go1.27.4", moduleLine()}},
		{"no version control", buildInfo("v0.0.1-test", false), []string{
			"version v0.0.1-test", "go go1.27.4", moduleLine()}},
		{"an empty version", buildInfo("", false), []string{
			"version (devel)", "go go1.27.4", moduleLine()}},
		{"no build info", nil, []string{
			"version (devel)", "go " + runtime.Version(), moduleLine()}},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got, want := cli.Version(c.bi, false), strings.Join(c.want, "\n")+"\n"; got != want {
				t.Fatalf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestVersionJSON is version's object: version, go, module and exit, and commit and commitTime
// when the build has them (spec/protocol.md, section 11, commands-1).
func TestVersionJSON(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{"a tag", buildInfo("v0.0.1-test", true),
			`{"commit":"` + testRevision + `","commitTime":"` + testTime + `","exit":0,"go":"go1.27.4","module":` + moduleJSON() + `,"version":"v0.0.1-test"}`},
		{"no version control", buildInfo("v0.0.1-test", false),
			`{"exit":0,"go":"go1.27.4","module":` + moduleJSON() + `,"version":"v0.0.1-test"}`},
		{"no build info", nil,
			`{"exit":0,"go":"` + runtime.Version() + `","module":` + moduleJSON() + `,"version":"(devel)"}`},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got, want := cli.Version(c.bi, true), c.want+"\n"; got != want {
				t.Fatalf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestVersionCommand runs version as the binary's own: what it prints on standard output is the
// build's, and nothing on standard error; with --json, one object.
func TestVersionCommand(t *testing.T) {
	t.Parallel()
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		bi = nil
	}
	r := run(t, env(), "version")
	if r.code != 0 || r.stderr != "" || r.stdout != cli.Version(bi, false) {
		t.Fatalf("version: %v", r)
	}
	r = run(t, env(), "version", "--json")
	if r.code != 0 || r.stderr != "" || r.stdout != cli.Version(bi, true) {
		t.Fatalf("version --json: %v", r)
	}
	r = run(t, env(), "version", "extra")
	if r.code != 3 || r.stdout != "" || r.stderr != "usage: placket version: bad flags\nerror usage\n" {
		t.Fatalf("version extra: %v", r)
	}
}

// TestVersionModuleMatchesCard: version names the same decision module as card --default.
func TestVersionModuleMatchesCard(t *testing.T) {
	t.Parallel()
	v := must(t, "version")
	c := must(t, "card", "--default")
	var fromVersion, fromCard string
	for _, l := range strings.Split(strings.TrimSuffix(v.stdout, "\n"), "\n") {
		if strings.HasPrefix(l, "module ") {
			fromVersion = l
		}
	}
	for _, l := range c.lines() {
		if strings.HasPrefix(l, "module ") {
			fromCard = l
		}
	}
	if fromVersion == "" || fromVersion != fromCard {
		t.Fatalf("version's module line %q, card --default's %q", fromVersion, fromCard)
	}
}
