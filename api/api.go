// Package api holds the mediator API's schemas, for a client in any language and for placket mcp's
// tool input schemas: the wire layer under wire/, a door call and its reply per op; the request
// layer under request/, what adapters/client and the MCP tools take; and the labels config's schema
// under config/. docs/reference/mediator-api.md is generated from them by go run ./api/gen, and
// core/conformance's TestAPISchemas compiles each and validates the worked calls, the recorded
// replies and the config fixtures under testdata/ against them. Every $id is under
// https://placket.tech/api/0.1/, and no schema names one outside this directory.
//
// The package holds the files only, and imports nothing of placket.
package api

import "embed"

// FS holds the schemas: wire/, request/ and config/.
//
//go:embed wire request config
var FS embed.FS

// Request returns the request schema of the tool name, request/<name>.json.
func Request(name string) ([]byte, error) { return FS.ReadFile("request/" + name + ".json") }
