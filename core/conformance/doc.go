// Package conformance is placket's conformance runner: it reads the vectors under vectors/, checks
// each against spec/schema/vector.json, runs each part through the core and compares every result,
// and holds the coverage list to the requirement ids and scenarios of spec/protocol.md.
//
// It implements the runner of conformance-vectors-1 to conformance-vectors-11: the order in which a
// vector fails (the check table of section 10), the header first (conformance-vectors-11), the
// schema at every depth (conformance-vectors-9, conformance-vectors-10), each byte input exactly once
// (conformance-vectors-8) and an operation run in full or not at all (conformance-vectors-4).
//
// A part is passed or failed; nothing is skipped. A part a handler cannot run yet is pending, and a
// pending part fails its vector, as does an operation no handler runs. Decision record 0004 says why
// the runner lives in the core and reads the schemas without a dependency.
package conformance
