package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/orilloai/placket/core/composition"
)

func init() {
	commands[LabelVersion] = versionCommand
}

// versionCommand is `version`: it prints which build the binary is, on standard output, since that is
// its result (spec/protocol.md, section 11, commands-1; docs/release.md).
func versionCommand(r *run, args []string) int {
	fs := flags(LabelVersion)
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		bi = nil
	}
	r.version(bi, composition.KernelVersion, composition.KernelHash())
	return ExitOK
}

// version prints and puts what names a build: the main module's version as Go stamped it, or
// (devel) when it stamped none; the commit and its time when the build carries them; the Go it was
// built with; and the decision module's version and hash. It reads nothing but its arguments, and
// the runtime's Go version when bi is nil.
func (r *run) version(bi *debug.BuildInfo, module, hash string) {
	v, goVersion := devel, runtime.Version()
	var commit, commitTime string
	if bi != nil {
		if bi.Main.Version != "" {
			v = bi.Main.Version
		}
		if bi.GoVersion != "" {
			goVersion = bi.GoVersion
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.time":
				commitTime = s.Value
			}
		}
	}
	fmt.Fprintln(r.stdout, LabelVersion+" "+v)
	r.put(LabelVersion, v)
	if commit != "" {
		fmt.Fprintln(r.stdout, LabelCommit+" "+commit)
		r.put(LabelCommit, commit)
	}
	if commitTime != "" {
		fmt.Fprintln(r.stdout, LabelCommitTime+" "+commitTime)
		r.put(LabelCommitTime, commitTime)
	}
	fmt.Fprintln(r.stdout, LabelGo+" "+goVersion)
	r.put(LabelGo, goVersion)
	fmt.Fprintln(r.stdout, LabelModule+" "+module+" "+hash)
	r.put(LabelModule, kernel{Version: module, Hash: hash})
}

// devel is the version of a build Go stamped none on.
const devel = "(devel)"
