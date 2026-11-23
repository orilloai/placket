// Package judge is the seam between the window and a judge: a model behind a network service that
// a maybe term lets pass or block a part where the terms would ask (composition-7). It is
// provider-neutral. It holds a judge's config, the interface every provider's adapter implements,
// the runners that ask a rule's judges, and the record each run leaves, and nothing of any one
// provider; adapters/judge/jev is this version's one adapter. Decision record 0009 says why.
//
// A judge's config is a door format, not a frozen one: one canonical JSON object of exactly
//
//	name         the judge's card name, a label
//	provider     the provider whose adapter runs it: jev
//	host         the one host a request may go to: a DNS name, with an optional :port
//	model        the provider's model
//	threshold    a whole number from 0 to 100; an answer whose score is below it abstains
//	instruction  what the judge is asked to decide, sent with every part
//
// Its typed hash is the card's hash for that judge (setup-contract-4), so any byte changed is a new
// card. A record's line, one per judge run in the journal's judged/ directory, is a door format too:
// {"abstained"?,"error"?,"judge","outcome","output"?,"outputBase64"?,"score"?,"use"}.
package judge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/setup"
)

// Budget is how long a rule's decide judges have, together, to settle a give, and how long each
// advise run has (docs/adr/0009-who-decided.md). A judge still running at the budget says none.
const Budget = 20 * time.Second

// MaxOutput is the most bytes of a provider's reply an adapter reads.
const MaxOutput = 64 << 10

// provider is the one provider of this version, whose adapter is adapters/judge/jev.
const provider = "jev"

// The outcomes of a run: a pass or a block decides, and none drops through to the next judge, and
// at the end of the stack to the people the rule names.
const (
	outcomePass  = string(composition.Pass)
	outcomeBlock = string(composition.Block)
	outcomeNone  = "none"
)

// The verdicts of an Answer.
const (
	verdictAbstain = "abstain"
)

// Code says why a judge said none, or why a config was refused.
type Code string

// The codes of a run that said none, each a record's error.
const (
	// CodeNoKey: the adapter has no key, and sent nothing.
	CodeNoKey Code = "no-key"
	// CodeTimeout: the budget ended the call.
	CodeTimeout Code = "timeout"
	// CodeUnreachable: the host could not be reached, or its TLS did not verify.
	CodeUnreachable Code = "unreachable"
	// CodeStatus: the host answered with a status that is not success.
	CodeStatus Code = "status"
	// CodeRedirect: the host answered with a redirect, which is never followed.
	CodeRedirect Code = "redirect"
	// CodeTooLarge: the reply was longer than MaxOutput.
	CodeTooLarge Code = "too-large"
	// CodeUnparsable: the reply did not read, or held a verdict or score the adapter cannot map.
	CodeUnparsable Code = "unparsable"
	// CodeRawBody: the part's body is raw, and is never sent to a judge.
	CodeRawBody Code = "raw-body"
)

// The codes of a refused config, in the order ReadConfig checks them (check-order).
const (
	// CodeConfigCanonical: the bytes are not in canonical form, a fraction included (canonical-5).
	CodeConfigCanonical Code = "config-canonical"
	// CodeConfigShape: a member is missing, unknown or of the wrong type, or the name is no label.
	CodeConfigShape Code = "config-shape"
	// CodeConfigProvider: the provider is not one this build has an adapter for.
	CodeConfigProvider Code = "config-provider"
	// CodeConfigHost: the host is not a DNS name with an optional port.
	CodeConfigHost Code = "config-host"
	// CodeConfigThreshold: the threshold is not a whole number from 0 to 100.
	CodeConfigThreshold Code = "config-threshold"
)

// Error is a judge that said none, or a refused config: its code (one-error-type). It holds nothing
// of the request, the reply or the config.
type Error struct{ Code Code }

// Error returns the package and the code.
func (e *Error) Error() string { return "judge: " + string(e.Code) }

// A Config is a judge's config, as ReadConfig read it.
type Config struct {
	Name, Provider, Host, Model, Instruction string
	Threshold                                int
}

// A Loaded is a config with the typed hash ReadConfig returned for it, the card's hash for that
// judge.
type Loaded struct {
	Config Config
	Hash   string
}

// ReadConfig reads a judge's config strictly, and returns it with its typed hash (canonical-3). It
// refuses, in order: bytes the canonical form refuses or that are not in it, a member missing,
// unknown or of the wrong type, a provider other than jev, a host that is not a DNS name with an
// optional port, and a threshold outside 0 to 100. Its callers trim one trailing line feed first.
func ReadConfig(b []byte) (Config, string, error) {
	fail := func(c Code) (Config, string, error) { return Config{}, "", &Error{Code: c} }
	c, err := canonical.Form(b)
	if err != nil || !bytes.Equal(c, b) {
		return fail(CodeConfigCanonical)
	}
	var w struct {
		Name        *string `json:"name"`
		Provider    *string `json:"provider"`
		Host        *string `json:"host"`
		Model       *string `json:"model"`
		Threshold   *int    `json:"threshold"`
		Instruction *string `json:"instruction"`
	}
	if !strict.Decode(b, &w) || w.Name == nil || w.Provider == nil || w.Host == nil || w.Model == nil ||
		w.Threshold == nil || w.Instruction == nil || !label(*w.Name) || *w.Model == "" || *w.Instruction == "" {
		return fail(CodeConfigShape)
	}
	if *w.Provider != provider {
		return fail(CodeConfigProvider)
	}
	if !host(*w.Host) {
		return fail(CodeConfigHost)
	}
	if *w.Threshold < 0 || *w.Threshold > 100 {
		return fail(CodeConfigThreshold)
	}
	h, err := canonical.TypedHash(b)
	if err != nil {
		return fail(CodeConfigCanonical)
	}
	return Config{Name: *w.Name, Provider: *w.Provider, Host: *w.Host, Model: *w.Model,
		Instruction: *w.Instruction, Threshold: *w.Threshold}, h, nil
}

// label reports whether s is a card name: lowercase letters and digits in words joined by single
// hyphens, at most 64 characters (setup-contract-4).
func label(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	join := true
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			join = false
		case c == '-' && !join:
			join = true
		default:
			return false
		}
	}
	return !join
}

// host reports whether s is a DNS name with an optional port: lowercase letters, digits and hyphens
// in labels of 1 to 63 bytes joined by dots, at most 253 bytes, at least two labels, no label that
// starts or ends with a hyphen, and a last label that is not all digits, so no IP literal; then
// optionally a colon and a port from 1 to 65535 with no leading zero. So it holds no scheme, path,
// user, query or IP literal.
func host(s string) bool {
	name, port, hasPort := strings.Cut(s, ":")
	if hasPort {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return false
		}
	}
	if name == "" || len(name) > 253 || net.ParseIP(name) != nil {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			if c := l[i]; (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	last := labels[len(labels)-1]
	return strings.Trim(last, "0123456789") != ""
}

// A Request is what a judge is asked about: the part's kind, the config's instruction, and the
// part's body, a text's text or a data body's canonical JSON. It holds a body, so it prints as a
// fixed text under every format verb (secrets-never-print).
type Request struct {
	Kind, Instruction string
	Body              []byte
}

// Format prints the Request as a fixed text, whatever the verb.
func (Request) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("judge.Request")) }

// GoString prints the Request as a fixed text.
func (Request) GoString() string { return "judge.Request" }

// String prints the Request as a fixed text.
func (Request) String() string { return "judge.Request" }

// An Answer is a judge's reply, as its adapter mapped it: Verdict pass, block or abstain; Score from
// 0 to 100, or nil when the reply carried none; and the model's text, which is content. It prints as
// a fixed text under every format verb.
type Answer struct {
	Verdict string
	Score   *int
	Output  []byte
}

// Format prints the Answer as a fixed text, whatever the verb.
func (Answer) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("judge.Answer")) }

// GoString prints the Answer as a fixed text.
func (Answer) GoString() string { return "judge.Answer" }

// String prints the Answer as a fixed text.
func (Answer) String() string { return "judge.Answer" }

// A Judge is one provider's adapter: it asks its judge about a request. It returns promptly once ctx
// is done, so a run ends when its budget does or the window stops. An error is the judge saying
// none; a *Error names why.
type Judge interface {
	Judge(ctx context.Context, r Request) (Answer, error)
}

// A Configured is one judge of a rule, its config, the use the rule grants it, and its adapter.
type Configured struct {
	Config Config
	Use    setup.Use
	Judge  Judge
}

// A Stack is a rule's judges, in the rule's order.
type Stack []Configured

// A Record is what one judge run left: the judge's name, its use, the outcome (pass, block or none),
// the error code that made it none, whether it abstained, its score and the model's output. The
// output is content, so a Record prints as a fixed text under every format verb.
type Record struct {
	Judge, Use, Outcome, Error string
	Abstained                  bool
	Score                      *int
	Output                     []byte
}

// Format prints the Record as a fixed text, whatever the verb.
func (Record) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("judge.Record")) }

// GoString prints the Record as a fixed text.
func (Record) GoString() string { return "judge.Record" }

// String prints the Record as a fixed text.
func (Record) String() string { return "judge.Record" }

// Run asks the stack's decide judges, in order, within budget for them all, each with its own
// config's instruction in place of r's, and settles on one judgment: the first pass or block, or nil
// when every judge said none (composition-7). Once a judge passes or blocks, no judge after it is
// asked. It returns a record of each judge it asked, and of each the budget left unasked.
func Run(ctx context.Context, s Stack, r Request, budget time.Duration) (*composition.Judgment, []Record) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var records []Record
	for _, c := range s {
		if c.Use != setup.Decide {
			continue
		}
		rec := ask(ctx, c, r)
		records = append(records, rec)
		if rec.Outcome == outcomePass || rec.Outcome == outcomeBlock {
			return &composition.Judgment{Name: c.Config.Name, Verdict: composition.Verdict(rec.Outcome)}, records
		}
	}
	return nil, records
}

// Advise asks the stack's advise judges, in order, each within a budget of its own, and returns a
// record of each. What they say decides nothing.
func Advise(ctx context.Context, s Stack, r Request, budget time.Duration) []Record {
	var records []Record
	for _, c := range s {
		if c.Use != setup.Advise {
			continue
		}
		actx, cancel := context.WithTimeout(ctx, budget)
		records = append(records, ask(actx, c, r))
		cancel()
	}
	return records
}

// ask asks one judge and records what it said. The budget's context ending the call is timeout,
// whatever the judge returned; a *Error is its own code; any other error is unreachable. A verdict
// of pass or block whose score is below the config's threshold abstains, and with no score the
// verdict stands. Abstaining, and every error, is none.
func ask(ctx context.Context, c Configured, r Request) Record {
	rec := Record{Judge: c.Config.Name, Use: string(c.Use), Outcome: outcomeNone}
	if ctx.Err() != nil {
		rec.Error = string(CodeTimeout)
		return rec
	}
	req := Request{Kind: r.Kind, Instruction: c.Config.Instruction, Body: append([]byte(nil), r.Body...)}
	a, err := c.Judge.Judge(ctx, req)
	var je *Error
	switch {
	case ctx.Err() != nil:
		rec.Error = string(CodeTimeout)
		return rec
	case errors.As(err, &je):
		rec.Error = string(je.Code)
		return rec
	case err != nil:
		rec.Error = string(CodeUnreachable)
		return rec
	}
	if a.Score != nil {
		s := *a.Score
		rec.Score = &s
	}
	rec.Output = append([]byte(nil), a.Output...)
	switch a.Verdict {
	case outcomePass, outcomeBlock:
		if a.Score != nil && *a.Score < c.Config.Threshold {
			rec.Abstained = true
			return rec
		}
		rec.Outcome = a.Verdict
	case verdictAbstain:
		rec.Abstained = true
	default:
		rec.Error, rec.Score, rec.Output = string(CodeUnparsable), nil, nil
	}
	return rec
}

// Line is a record's line in canonical JSON. Output that is valid UTF-8 and that the canonical form
// takes is output; any other is standard base64 in outputBase64, so the line stays canonical.
func Line(r Record) []byte {
	m := map[string]any{"judge": r.Judge, "outcome": r.Outcome, "use": r.Use}
	if r.Abstained {
		m["abstained"] = true
	}
	if r.Error != "" {
		m["error"] = r.Error
	}
	if r.Score != nil {
		m["score"] = *r.Score
	}
	if len(r.Output) > 0 && utf8.Valid(r.Output) {
		m["output"] = string(r.Output)
		if b := encode(m); b != nil {
			return b
		}
		delete(m, "output")
	}
	if len(r.Output) > 0 {
		m["outputBase64"] = base64.StdEncoding.EncodeToString(r.Output)
	}
	return encode(m)
}

// encode is v's canonical bytes, or nil when the canonical form refuses them.
func encode(v any) []byte {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil
	}
	c, err := canonical.Form(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}))
	if err != nil {
		return nil
	}
	return c
}
