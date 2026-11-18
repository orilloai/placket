package mcp

// Code is the code of a tool call placket mcp refused on its own side.
type Code string

// The codes of Error.
const (
	// CodeAnsweringKey: the label's key holds a role a maybe term's resolve.to names, so it answers
	// questions under its Accord, and placket mcp serves proposing keys only.
	CodeAnsweringKey Code = "answering-key"
	// CodeFounding: the label's founding set cannot be read or does not found.
	CodeFounding Code = "founding"
	// CodeBadArguments: the tool's arguments fail its input schema, or the wait cap.
	CodeBadArguments Code = "bad-arguments"
	// CodeInternal: a failure no other code names.
	CodeInternal Code = "internal"
)

// Error is a tool call refused on placket mcp's side: its code. It holds nothing of the call.
type Error struct{ Code Code }

// Error returns "mcp: " and the code.
func (e *Error) Error() string { return "mcp: " + string(e.Code) }
