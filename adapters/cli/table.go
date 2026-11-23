package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The command table: one row per command and subcommand, which says what the command does, what it
// takes and what its --json object follows. Each command reads its flags from its row (newFlags), so
// a flag cannot be read without the row naming it, and docs/reference/cli.md is generated from the
// table (go run ./adapters/cli/gen). The rows are data: what a command does with a flag is its own.

// A Row is one command of the table.
type Row struct {
	// Words name the command: one word, or two for a subcommand, such as key new.
	Words []string
	// Summary says what the command does, in one sentence.
	Summary string
	// Usage is what follows placket and the words: each argument in angle brackets, each flag with
	// its value, an optional one in square brackets, and <socket flags> for the flags every socket
	// command takes. --json is left out.
	Usage string
	// Args are the command's positional arguments, in order.
	Args []Arg
	// Flags are every flag the command reads, in the order a reader takes them.
	Flags []Flag
	// JSON are the ids of the requirements of spec/protocol.md, section 11, that the command's --json
	// object follows; none for a command that takes no --json.
	JSON []string
	// Exits are the exit codes the command can end with, in order.
	Exits []int
}

// An Arg is a positional argument of a command.
type Arg struct {
	// Name is the argument as the usage writes it, without its angle brackets.
	Name string
	// Values are the values it takes, in the order they are offered.
	Values   []string
	Sentence string
}

// A Kind is what a flag holds.
type Kind int

// The kinds of a flag.
const (
	// KindText holds one value, the last given.
	KindText Kind = iota
	// KindNumber holds one whole number.
	KindNumber
	// KindSwitch holds no value: it is given or not.
	KindSwitch
	// KindList holds every value given, in order; it may be given more than once.
	KindList
)

func (k Kind) String() string {
	switch k {
	case KindNumber:
		return "number"
	case KindSwitch:
		return "switch"
	case KindList:
		return "list"
	}
	return "text"
}

// A Flag is one flag of a command.
type Flag struct {
	Name string
	Kind Kind
	// Value is what the flag takes, as the usage writes it, such as <file>; empty for a switch.
	Value string
	// Default is the value the flag holds when it is not given, as the flag package prints it; empty
	// for its kind's zero value.
	Default string
	// DefaultArg is set on a flag whose default is the value given to the row's first argument, such
	// as init's --label, whose default is the example's name. Default is then empty.
	DefaultArg bool
	// Required is a flag the command needs, and refuses its run without as error usage. A socket
	// command's --key, --socket and --accord are not: --label may take their place.
	Required bool
	// Own is set on a required flag the command checks itself, with its value, so that a run
	// without it ends as the command says, such as decline's error comment, and not with the line
	// `--x is required`.
	Own      bool
	Sentence string
}

// An Exit is an exit code and what it means.
type Exit struct {
	Code     int
	Sentence string
}

// Exits are the command line's exit codes, in order.
func Exits() []Exit {
	return []Exit{
		{ExitOK, "A pass, or a command that did what it was asked."},
		{ExitBlock, "A block on `give`, `answer` or `change`; a release that blocked; a `give --wait` or `result` whose question closed any way but an accept that passed and was released; a comment that did not cross; a file, or a value inside it, that no release matches; a log that fails verification in `verify`: one that is not valid, holds more than one Accord or is not the one a successor supersedes, each with `error verify` and its code on standard error, or one that does not conform; and a body that is not its receipt's part, in `verify`."},
		{ExitRejected, "A rejection: the window refused the call, and standard error ends `rejected` and its code."},
		{ExitTool, "A tool or door error: standard error ends `error`, or `door`, and its code. `error verify` from `verify` is a log that fails, exit 1."},
		{ExitOpen, "An ask still open: the question's id is on standard output."},
	}
}

// Table is every row of the command table, in the order the reference gives them. It is a copy.
func Table() []Row {
	out := make([]Row, len(table))
	for i, r := range table {
		r.Words = append([]string(nil), r.Words...)
		r.Args = append([]Arg(nil), r.Args...)
		for j := range r.Args {
			r.Args[j].Values = append([]string(nil), r.Args[j].Values...)
		}
		r.Flags = append([]Flag(nil), r.Flags...)
		r.JSON = append([]string(nil), r.JSON...)
		r.Exits = append([]int(nil), r.Exits...)
		out[i] = r
	}
	return out
}

// The flags most commands share.
var (
	jsonRow = Flag{Name: flagJSON, Kind: KindSwitch, Sentence: "Print the result as one JSON object on standard output."}
	// socketRows are the flags every socket command takes, first in its row.
	socketRows = []Flag{
		{Name: "key", Value: "<file>", Sentence: "The key file of the member the command speaks for; needed unless `--label` is given."},
		{Name: "socket", Value: "<path>", Sentence: "The window's socket; needed unless `--label` is given."},
		{Name: "accord", Value: "<id>", Sentence: "The Accord id the window serves; needed unless `--label` is given."},
		{Name: "label", Value: "<label>", Sentence: "A mediator of the labels config, by its label, or by its Accord id when it has none, in place of `--key`, `--socket` and `--accord`."},
		{Name: "state", Value: "<dir>", Sentence: "The member's state directory; by default `<key file>.state/<the Accord id's hex>/`."},
	}
	questionRow = Flag{Name: "question", Value: "<id>", Required: true, Sentence: "The question's id: the receipt id of its ask."}
)

// socketRow is a socket command's flags: the socket flags, the command's own, and --json.
func socketRow(own ...Flag) []Flag {
	out := append(append([]Flag(nil), socketRows...), own...)
	return append(out, jsonRow)
}

// withJSON is a command's flags and --json.
func withJSON(own ...Flag) []Flag { return append(append([]Flag(nil), own...), jsonRow) }

var table = []Row{
	{Words: []string{"init"}, Summary: "Found an example Accord for one person, who holds every key, and print what to run next.",
		Usage: "<example> [--dir <dir>] [--label <label>]",
		Args:  []Arg{{Name: "example", Values: exampleNames, Sentence: "The entry point to set up: approve before apply, the out-gate of a merge, or an agent handoff."}},
		Flags: withJSON(
			Flag{Name: "dir", Value: "<dir>", Sentence: "The directory to write the example in, empty or new; by default `<placket config dir>/<label>/`."},
			Flag{Name: "label", Value: "<label>", DefaultArg: true, Sentence: "The label the example's entries of the labels config start with; by default the example's name."}),
		JSON: []string{"commands-1"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"key"}, Summary: "Make a key file, or print a key file's actor reference.",
		Usage: "<subcommand>",
		Args:  []Arg{{Name: "subcommand", Values: []string{"new", "ref"}, Sentence: "What to do: `key new` or `key ref`."}},
		Flags: withJSON(),
		JSON:  []string{"commands-1"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"key", "new"}, Summary: "Write a new key file, mode 0600, and print its actor reference; it never prints the seed.",
		Usage: "--out <file>",
		Flags: withJSON(Flag{Name: "out", Value: "<file>", Required: true, Sentence: "The key file to write; a file already there is never replaced."}),
		JSON:  []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"key", "ref"}, Summary: "Print the actor reference of a key file.",
		Usage: "--key <file>",
		Flags: withJSON(Flag{Name: "key", Value: "<file>", Required: true, Sentence: "The key file to read."}),
		JSON:  []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"declare"}, Summary: "Sign the key's declaration of its terms and print it.",
		Usage: "--key <file> --terms <file>",
		Flags: withJSON(
			Flag{Name: "key", Value: "<file>", Required: true, Sentence: "The key file of the party that declares."},
			Flag{Name: "terms", Value: "<file>", Required: true, Sentence: "A file holding the terms, one JSON object; founding checks the terms themselves."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"seal"}, Summary: "Seal the key's own signed declaration: print the seal entry for founding, and write the opening for whoever runs the window.",
		Usage: "--key <file> --declaration <file> [--out <dir>]",
		Flags: withJSON(
			Flag{Name: "key", Value: "<file>", Required: true, Sentence: "The key file that signed the declaration."},
			Flag{Name: "declaration", Value: "<file>", Required: true, Sentence: "The signed declaration, as `declare` prints it."},
			Flag{Name: "out", Value: "<dir>", Default: ".", Sentence: "The directory the opening is written to, as `<hex of the actor reference>.opening.json`, mode 0600; a file already there is never replaced."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"bind"}, Summary: "Sign, as a principal, a binding that gives an actor its roles under one mediator and one policy until a time, and print it.",
		Usage: "--key <file> --actor <actor> --mediator <actor> --until <time> --role <role>... --policy <file>",
		Flags: withJSON(
			Flag{Name: "key", Value: "<file>", Required: true, Sentence: "The principal's key file, which signs the binding."},
			Flag{Name: "actor", Value: "<actor>", Required: true, Sentence: "The actor reference the binding is for, as `key ref` prints it."},
			Flag{Name: "mediator", Value: "<actor>", Required: true, Sentence: "The mediator's actor reference, which the binding names."},
			Flag{Name: "until", Value: "<time>", Required: true, Sentence: "When the binding ends: a timestamp in whole seconds, such as 2099-01-01T00:00:00Z."},
			Flag{Name: "role", Kind: KindList, Value: "<role>", Required: true, Sentence: "A role the binding gives, such as `decider`; given once per role."},
			Flag{Name: "policy", Value: "<file>", Required: true, Sentence: "The founding set's policy, whose typed hash the binding names."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"founding"}, Summary: "Put a policy, a mediator, a card, the seals and the bindings in one founding set, check that it founds, and print it with its Accord id and card hash.",
		Usage: "--policy <file> --mediator <file> --card <file> --seal <file>... --binding <file>...",
		Flags: withJSON(
			Flag{Name: "policy", Value: "<file>", Required: true, Sentence: "The policy: which roles each principal may grant."},
			Flag{Name: "mediator", Value: "<file>", Required: true, Sentence: "A file holding the mediator: its actor reference, its operator and how it runs."},
			Flag{Name: "card", Value: "<file>", Required: true, Sentence: "The card, as `card` prints it."},
			Flag{Name: "seal", Kind: KindList, Value: "<file>", Required: true, Sentence: "A declaration's seal entry, as `seal` prints it; given once per party."},
			Flag{Name: "binding", Kind: KindList, Value: "<file>", Required: true, Sentence: "A binding, as `bind` prints it; given once per binding."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"card"}, Summary: "Print this build's card, with the judges' configs given, and its card hash and the module it names.",
		Usage: "--default [--judge <file>]...",
		Flags: withJSON(
			Flag{Name: "default", Kind: KindSwitch, Required: true, Sentence: "Print this build's card, the one card the command line builds."},
			Flag{Name: "judge", Kind: KindList, Value: "<file>", Sentence: "A judge's config, given once per judge; the card lists them in this order."}),
		JSON: []string{"commands-1"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"window"}, Summary: "Run the mediator of an Accord behind its door, on a unix socket, with its journal, until it is interrupted.",
		Usage: "--key <file> --founding <file> --openings <dir> [--acceptances <file>] [--socket <path>] [--journal <dir>] [--judge <file>]...",
		Flags: withJSON(
			Flag{Name: "key", Value: "<file>", Required: true, Sentence: "The mediator's key file."},
			Flag{Name: "founding", Value: "<file>", Required: true, Sentence: "The founding set, as `founding` prints it."},
			Flag{Name: "openings", Value: "<dir>", Required: true, Sentence: "The directory of every member's opening, each as `seal` writes it."},
			Flag{Name: "socket", Value: "<path>", Sentence: "The socket to listen on; by default the Accord's own under the runtime directory."},
			Flag{Name: "journal", Value: "<dir>", Sentence: "The journal's directory; by default the Accord's own under placket's state directory."},
			Flag{Name: "acceptances", Value: "<file>", Sentence: "The members' acceptances; by default the `acceptances.json` beside the founding set, which the window writes once every member has accepted."},
			Flag{Name: "judge", Kind: KindList, Value: "<file>", Sentence: "A judge's config the card lists, given once per judge; a window needs exactly the card's."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"window", "close"}, Summary: "Close the Accord of a stopped window's journal: close each open question, write the close receipt last, and with `--socket` serve the closed Accord.",
		Usage: "--key <file> --founding <file> --journal <dir> --accord <id> [--acceptances <file>] [--openings <dir>] [--socket <path>]",
		Flags: withJSON(
			Flag{Name: "key", Value: "<file>", Required: true, Sentence: "The mediator's key file."},
			Flag{Name: "founding", Value: "<file>", Required: true, Sentence: "The founding set of the Accord to close."},
			Flag{Name: "journal", Value: "<dir>", Required: true, Sentence: "The journal to close, of a window that is stopped."},
			Flag{Name: "accord", Value: "<id>", Required: true, Sentence: "The founding set's Accord id, given as a guard, since a close cannot be undone."},
			Flag{Name: "socket", Value: "<path>", Sentence: "Serve the closed Accord on this socket until interrupted; without it, the command exits once the close is written."},
			Flag{Name: "openings", Value: "<dir>", Sentence: "The directory of the members' openings; a close decides nothing, so it needs none."},
			Flag{Name: "acceptances", Value: "<file>", Sentence: "The members' acceptances, as `window` takes them."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"view"}, Summary: "Ask the window for the key's own view of a proposed Accord, keep it in the state directory, and print it.",
		Usage: "<socket flags>",
		Flags: socketRow(),
		JSON:  []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitRejected, ExitTool}},
	{Words: []string{"accept"}, Summary: "Sign the key's acceptance of the view its state directory keeps, hand it to the window, and say whether the Accord is now founded.",
		Usage: "<socket flags>",
		Flags: socketRow(),
		JSON:  []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitRejected, ExitTool}},
	{Words: []string{"give"}, Summary: "Give another member one part, and print the outcome: pass, block, or ask, with the question's id.",
		Usage: "<socket flags> --to <actor> --kind <kind> (--text <text> | --file <file> [--as <form>]) [--re <id>] [--topic <topic>] [--keep] [--wait]",
		Flags: socketRow(
			Flag{Name: "to", Value: "<actor>", Required: true, Sentence: "The receiver: a member's actor reference."},
			Flag{Name: "kind", Value: "<kind>", Required: true, Sentence: "The part's kind, which the terms select on."},
			Flag{Name: "text", Value: "<text>", Sentence: "The body, as text; one of `--text` and `--file` is needed."},
			Flag{Name: "file", Value: "<file>", Sentence: "A file holding the body, read as `--as` says."},
			Flag{Name: "as", Value: "<form>", Default: "text", Sentence: "How `--file` is read: `text`, `raw` or `data`, data being one JSON value in canonical form."},
			Flag{Name: "re", Value: "<id>", Sentence: "What the part is about, such as a question's id."},
			Flag{Name: "topic", Value: "<topic>", Sentence: "The part's topic."},
			Flag{Name: "keep", Kind: KindSwitch, Sentence: "Keep the part given in the state directory's `kept.jsonl`."},
			Flag{Name: "wait", Kind: KindSwitch, Sentence: "When the part is asked about, wait until its question closes, holding no lock, and print what `result` prints."}),
		JSON: []string{"commands-1", "commands-2"}, Exits: []int{ExitOK, ExitBlock, ExitRejected, ExitTool, ExitOpen}},
	{Words: []string{"questions"}, Summary: "Print the open questions this key may answer, one JSON object per line.",
		Usage: "<socket flags> [--wait <seconds>]",
		Flags: socketRow(Flag{Name: "wait", Kind: KindNumber, Value: "<seconds>", Sentence: "Wait at most this many seconds, up to 60, until there is a question to show."}),
		JSON:  []string{"commands-1"}, Exits: []int{ExitOK, ExitRejected, ExitTool}},
	{Words: []string{"watch"}, Summary: "Wait on the questions of one or more mediators and print each new question once, as it opens, with the lines that answer it; it never answers.",
		Usage: "--label <label>... [--count <n>]",
		Flags: []Flag{
			{Name: "label", Kind: KindList, Value: "<label>", Required: true, Own: true, Sentence: "A mediator of the labels config to watch, given once per mediator; at least one is needed."},
			{Name: "count", Kind: KindNumber, Value: "<n>", Sentence: "Exit once this many questions are printed; without it, run until interrupted."},
			{Name: flagJSON, Kind: KindSwitch, Sentence: "Print one JSON object per question on standard output, each on a line of its own as the question opens, and then the result as one object."}},
		JSON: []string{"commands-1"}, Exits: []int{ExitOK, ExitRejected, ExitTool}},
	{Words: []string{"answer"}, Summary: "Accept a question, asking for release unless `--no-release`, and print the answer's outcome, then the release's.",
		Usage: "<socket flags> --question <id> [--text <text>] [--no-release] [--keep]",
		Flags: socketRow(questionRow,
			Flag{Name: "text", Value: "<text>", Default: WordAccepted, Sentence: "The answer's text."},
			Flag{Name: "no-release", Kind: KindSwitch, Sentence: "Accept with no release: the part does not pass on."},
			Flag{Name: "keep", Kind: KindSwitch, Sentence: "Keep the answer call signed, and the salt minted, in `kept.jsonl`."}),
		JSON: []string{"commands-1", "commands-2"}, Exits: []int{ExitOK, ExitBlock, ExitRejected, ExitTool}},
	{Words: []string{"change"}, Summary: "Answer a question with a changed version, which passes on and goes back to the asker, and print both outcomes.",
		Usage: "<socket flags> --question <id> (--text <text> | --file <file>) [--comment <text>] [--keep] [--no-release]",
		Flags: socketRow(questionRow,
			Flag{Name: "text", Value: "<text>", Sentence: "The version, as text; one of `--text` and `--file` is needed."},
			Flag{Name: "file", Value: "<file>", Sentence: "A file holding the version, as UTF-8 text."},
			Flag{Name: "comment", Value: "<text>", Sentence: "A comment given to the asker after the change."},
			Flag{Name: "keep", Kind: KindSwitch, Sentence: "Keep the answer call signed, the salt minted and the version in `kept.jsonl`."},
			Flag{Name: "no-release", Kind: KindSwitch, Sentence: "Change with no release: the version goes back to the asker alone."}),
		JSON: []string{"commands-1", "commands-2"}, Exits: []int{ExitOK, ExitBlock, ExitRejected, ExitTool}},
	{Words: []string{"decline"}, Summary: "Decline a question, then give the asker the comment, and say whether the comment crossed.",
		Usage: "<socket flags> --question <id> --comment <text>",
		Flags: socketRow(questionRow,
			Flag{Name: "comment", Value: "<text>", Required: true, Own: true, Sentence: "The comment a decline needs: why, for the asker."}),
		JSON: []string{"commands-1", "commands-2"}, Exits: []int{ExitOK, ExitBlock, ExitRejected, ExitTool}},
	{Words: []string{"cancel"}, Summary: "Cancel, as its asker, a question of the key's own, and print the outcome.",
		Usage: "<socket flags> --question <id>",
		Flags: socketRow(questionRow),
		JSON:  []string{"commands-1", "commands-2"}, Exits: []int{ExitOK, ExitRejected, ExitTool}},
	{Words: []string{"result"}, Summary: "Print how a question the key asked closed, its release, and the answer or version it fetched, as `give --wait` does.",
		Usage: "<socket flags> --question <id> [--wait <seconds>]",
		Flags: socketRow(questionRow,
			Flag{Name: "wait", Kind: KindNumber, Value: "<seconds>", Sentence: "Wait at most this many seconds, up to 60, holding no lock, while the question is open."}),
		JSON: []string{"commands-1", "commands-2"}, Exits: []int{ExitOK, ExitBlock, ExitRejected, ExitTool, ExitOpen}},
	{Words: []string{"get"}, Summary: "Fetch the next delivery queued for the key, or every one with `--all`, and print its body or the file it was written to.",
		Usage: "<socket flags> [--all] [--out <dir>] [--keep] [--wait <seconds>]",
		Flags: socketRow(
			Flag{Name: "out", Value: "<dir>", Sentence: "Write each body to a file of this directory, and print its path in place of the body."},
			Flag{Name: "keep", Kind: KindSwitch, Sentence: "Keep each delivery in `kept.jsonl`."},
			Flag{Name: "all", Kind: KindSwitch, Sentence: "Fetch every delivery waiting, in order, under one state lock."},
			Flag{Name: "wait", Kind: KindNumber, Value: "<seconds>", Sentence: "Wait at most this many seconds, up to 60, holding no lock, until a delivery is queued."}),
		JSON: []string{"commands-1", "commands-3"}, Exits: []int{ExitOK, ExitRejected, ExitTool}},
	{Words: []string{"mediators"}, Summary: "Print the labels config's entries, one JSON object per line, in the order the file lists them.",
		Usage: "",
		Flags: withJSON(),
		JSON:  []string{"commands-1"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"mcp"}, Summary: "Serve the mediators of the labels given to an agent as MCP tools, on standard input and output, as one party's client.",
		Usage: "[--label <label>]... [--allow-answering]",
		Flags: []Flag{
			{Name: "label", Kind: KindList, Value: "<label>", Sentence: "A mediator of the labels config to serve, given once per mediator."},
			{Name: "allow-answering", Kind: KindSwitch, Sentence: "Serve a label whose key answers questions too; by default such a label is refused."}},
		Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"verify"}, Summary: "Check a journal's receipts for validity under the mediator's key and, with a founding set, for conformance to the Accord, and print what was checked and what is not claimed.",
		Usage: "--receipts <dir> (--mediator <actor> | --accord <id> --founding <file>) [--founding <file>] [--acceptances <file>] [--answers <file>] [--subset] [--delivery <file>] [--superseded-by <file> --successor-accord <id> --successor-acceptances <file>] [--open <file>]... [--view <file>]",
		Flags: withJSON(
			Flag{Name: "receipts", Value: "<dir>", Required: true, Sentence: "The journal's directory, whose `receipts.jsonl` is checked."},
			Flag{Name: "mediator", Value: "<actor>", Sentence: "The mediator's actor reference, under which the receipts are checked; one of `--mediator` and `--accord` is needed."},
			Flag{Name: "accord", Value: "<id>", Sentence: "The Accord id, the one value given out of band: the mediator's reference comes from the founding set whose typed hash it is."},
			Flag{Name: "founding", Value: "<file>", Sentence: "The founding set, for conformance to the Accord; needed with `--accord`."},
			Flag{Name: "acceptances", Value: "<file>", Sentence: "The founding set's acceptances; by default the `acceptances.json` beside it."},
			Flag{Name: "answers", Value: "<file>", Sentence: "A file of answer calls, one a line, read beside the journal's `answers.jsonl` to witness answers."},
			Flag{Name: "subset", Kind: KindSwitch, Sentence: "The receipts are a subset, whose chains may start late; completeness is then not claimed."},
			Flag{Name: "delivery", Value: "<file>", Sentence: "A delivery, as `get --json` saved it, whose body and salt are checked against its receipt."},
			Flag{Name: "superseded-by", Value: "<file>", Sentence: "The founding set of the Accord that supersedes this one, whose supersedes must pin this journal's close; it needs `--accord`, `--founding`, `--successor-accord` and `--successor-acceptances`."},
			Flag{Name: "successor-accord", Value: "<id>", Sentence: "The successor's Accord id, from your own record, such as the id you accepted, never from a file sent with the journal; the set given with `--superseded-by` must be the one whose typed hash it is."},
			Flag{Name: "successor-acceptances", Value: "<file>", Sentence: "The successor's acceptances, with which it is founded at the time of this journal's close, so every member must have accepted it; never read from beside its founding set."},
			Flag{Name: flagOpen, Kind: KindList, Value: "<file>", Sentence: "A member's opening, given once per opening, so its seal and its terms are checked."},
			Flag{Name: flagView, Value: "<file>", Sentence: "One member's own view, as it kept it, to check that member's receipts against; it needs `--founding`."}),
		JSON: []string{"commands-1", "commands-4", "commands-8"}, Exits: []int{ExitOK, ExitBlock, ExitTool}},
	{Words: []string{"show"}, Summary: "Decode one receipt without verifying it, and print what it says and what it claims.",
		Usage: "--receipt <file>",
		Flags: withJSON(Flag{Name: "receipt", Value: "<file>", Required: true, Sentence: "A file holding one receipt."}),
		JSON:  []string{"commands-1", "commands-4"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"log"}, Summary: "Export a verified journal as a log, one row per question, and with `--all` one for every crossing.",
		Usage: "--mediator <actor> --receipts <dir> [--kept <dir>]... [--format <format>] [--words | --no-words] [--all]",
		Flags: withJSON(
			Flag{Name: "mediator", Value: "<actor>", Required: true, Sentence: "The mediator's actor reference, under which the receipts are checked."},
			Flag{Name: "receipts", Value: "<dir>", Required: true, Sentence: "The journal's directory."},
			Flag{Name: "format", Value: "<format>", Default: "json", Sentence: "How the log is printed: `json` or `csv`."},
			Flag{Name: "words", Kind: KindSwitch, Sentence: "Add the texts of the parts the state directories kept."},
			Flag{Name: "no-words", Kind: KindSwitch, Sentence: "Empty the comments."},
			Flag{Name: "all", Kind: KindSwitch, Sentence: "A row for every crossing, besides one per question."},
			Flag{Name: "kept", Kind: KindList, Value: "<dir>", Sentence: "A state directory whose kept parts the log reads, given once per directory."}),
		JSON: []string{"commands-1", "commands-4"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{"reconcile"}, Summary: "Reconcile a journal's receipts with what each state directory kept, and print what was removed, unrecorded or missing.",
		Usage: "--mediator <actor> --accord <id> --card <hash> --receipts <dir> --kept <dir>...",
		Flags: withJSON(
			Flag{Name: "mediator", Value: "<actor>", Required: true, Sentence: "The mediator's actor reference, under which the receipts are checked."},
			Flag{Name: "accord", Value: "<id>", Required: true, Sentence: "The Accord id."},
			Flag{Name: "card", Value: "<hash>", Required: true, Sentence: "The card hash."},
			Flag{Name: "receipts", Value: "<dir>", Required: true, Sentence: "The journal's directory."},
			Flag{Name: "kept", Kind: KindList, Value: "<dir>", Required: true, Sentence: "A state directory, given once per directory."}),
		JSON: []string{"commands-1", "commands-4"}, Exits: []int{ExitOK, ExitTool}},
	{Words: []string{WordReleased}, Summary: "Say whether a file, a value inside it, or a part the key kept, is a part that crossed to the state directory's key, and under which receipt.",
		Usage: "--state <dir> [--file <file>] [--kind <kind>]... [--at <pointer> (--value-file <file> | --value <text>)] [--re <id>] [--from <actor>] [--every]",
		Flags: withJSON(
			Flag{Name: "state", Value: "<dir>", Required: true, Sentence: "The state directory of the key the part crossed to."},
			Flag{Name: "file", Value: "<file>", Sentence: "A file whose bytes are checked; without it, every part the key kept is tried."},
			Flag{Name: "at", Value: "<pointer>", Sentence: "A JSON Pointer into the body, where the value is checked; it needs one of `--value-file` and `--value`."},
			Flag{Name: "value-file", Value: "<file>", Sentence: "A file holding the value's bytes."},
			Flag{Name: "value", Value: "<text>", Sentence: "The value, as text."},
			Flag{Name: "re", Value: "<id>", Sentence: "Match only a kept part whose `re` is this; without `--file` only."},
			Flag{Name: "from", Value: "<actor>", Sentence: "Match only a receipt from this sender."},
			Flag{Name: "every", Kind: KindSwitch, Sentence: "Print every match, in fetch order, not only the first; without `--file` only."},
			Flag{Name: "kind", Kind: KindList, Value: "<kind>", Sentence: "Match only a receipt of this kind, given once per kind."}),
		JSON: []string{"commands-1", "commands-5", "commands-6", "commands-7"}, Exits: []int{ExitOK, ExitBlock, ExitTool}},
	{Words: []string{LabelVersion}, Summary: "Print which build the binary is: its version, commit, Go release and decision module.",
		Usage: "",
		Flags: withJSON(),
		JSON:  []string{"commands-1"}, Exits: []int{ExitOK, ExitTool}},
}

// rowOf is the row of the command name, its words joined by a space, or nil.
func rowOf(name string) *Row {
	for i := range table {
		if strings.Join(table[i].Words, " ") == name {
			return &table[i]
		}
	}
	return nil
}

// A flagSet is the flag set of one command, whose flags are made from its row as the command reads
// them. A flag its row does not list is still defined, with no sentence, and TestTheCommandTable
// fails on it.
type flagSet struct {
	*flag.FlagSet
	row *Row
	// arg is the value of the row's first argument the name holds, such as approve in init approve;
	// empty when the name is the row's own.
	arg string
}

// newFlags makes the flag set of the command name, which prints nothing of its own, with --json when
// its row lists it.
func newFlags(name string) *flagSet { return newFlagsNamed(name, name) }

// newFlagsNamed makes the flag set of the row named row, under the name a person typed, such as
// init approve for the row init, whose argument is not a command of its own.
func newFlagsNamed(name, row string) *flagSet {
	fs := &flagSet{FlagSet: flag.NewFlagSet(name, flag.ContinueOnError), row: rowOf(row)}
	if name != row {
		fs.arg = strings.TrimPrefix(name, row+" ")
	}
	fs.SetOutput(io.Discard)
	if _, ok := fs.rowFlag(flagJSON); ok {
		fs.switchFlag(flagJSON)
	}
	return fs
}

// rowFlag is the row's flag of that name.
func (fs *flagSet) rowFlag(name string) (Flag, bool) {
	if fs.row != nil {
		for _, f := range fs.row.Flags {
			if f.Name == name {
				return f, true
			}
		}
	}
	return Flag{Name: name}, false
}

// text reads a flag that holds one value, by default its row's default, or the argument's value
// for a flag whose default is the argument's.
func (fs *flagSet) text(name string) *string {
	f, _ := fs.rowFlag(name)
	def := f.Default
	if f.DefaultArg {
		def = fs.arg
	}
	return fs.String(name, def, f.Sentence)
}

// number reads a flag that holds one whole number.
func (fs *flagSet) number(name string) *int {
	f, _ := fs.rowFlag(name)
	n, _ := strconv.Atoi(f.Default)
	return fs.Int(name, n, f.Sentence)
}

// switchFlag reads a flag that is given or not.
func (fs *flagSet) switchFlag(name string) *bool {
	f, _ := fs.rowFlag(name)
	return fs.Bool(name, f.Default == "true", f.Sentence)
}

// list reads a flag that may be given more than once into m.
func (fs *flagSet) list(m *many, name string) {
	f, _ := fs.rowFlag(name)
	fs.Var(m, name, f.Sentence)
}

// required are the flags of the row the command needs, in the row's order, but those named and
// those it checks itself.
func (fs *flagSet) required(but ...string) []string {
	if fs.row == nil {
		return nil
	}
	var out []string
	for _, f := range fs.row.Flags {
		if f.Required && !f.Own && !contains(but, f.Name) {
			out = append(out, f.Name)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// String is the row's flag as the reference writes it: its name and its value.
func (f Flag) String() string {
	if f.Value == "" {
		return "--" + f.Name
	}
	return fmt.Sprintf("--%s %s", f.Name, f.Value)
}
