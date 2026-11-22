package cli

import (
	"runtime/debug"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/mcp"
)

func init() {
	commands["mcp"] = mcpCommand
}

// mcpCommand is `mcp [--label L]... [--allow-answering]`: placket mcp, an MCP server on standard
// input and output, one party's client of the mediators the labels it is started with name
// (adapters/mcp). It serves until its input ends or it is interrupted, and exits 0. Standard output
// carries the protocol alone, so it takes no --json, and it prints what it serves, and what it
// refuses, on standard error. A label the config does not name is client error unknown-mediator, and
// a config that is refused is its client error, before anything is served.
func mcpCommand(r *run, args []string) int {
	fs := newFlags("mcp")
	var labels many
	fs.list(&labels, "label")
	allow := fs.switchFlag("allow-answering")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	cfg, err := r.config()
	if err != nil {
		return r.failed(err)
	}
	v := devel
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		v = bi.Main.Version
	}
	s := &mcp.Server{Config: cfg, Labels: labels, AllowAnswering: *allow, Version: v, Stderr: r.stderr,
		Options: client.Options{Now: r.env.Now, Rand: r.env.Rand, Stderr: r.stderr}}
	if err := s.Serve(r.env.Context, r.stdin, r.realOut); err != nil {
		return r.failed(err)
	}
	return ExitOK
}
