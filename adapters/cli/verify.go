package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

func init() {
	commands["verify"] = verifyCommand
}

// predicateOf is the predicate member of a receipt's statement, as the receipt holds it.
func predicateOf(receipt []byte) []byte {
	var env struct {
		Payload string `json:"payload"`
	}
	if json.Unmarshal(receipt, &env) != nil {
		return nil
	}
	p, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil
	}
	var st struct {
		Predicate json.RawMessage `json:"predicate"`
	}
	if json.Unmarshal(p, &st) != nil {
		return nil
	}
	return canonicalJSON(st.Predicate)
}

// notVerified prints the verifier's failure, its code and the line of receipts.jsonl it is at.
func (r *run) notVerified(err error) int {
	var re *receipts.Error
	if errors.As(err, &re) {
		if re.Index >= 0 {
			return r.tool(codeVerify, string(re.Code), journal.FileReceipts, fmt.Sprintf("line %d", re.Index+1))
		}
		return r.tool(codeVerify, string(re.Code))
	}
	return r.tool(codeVerify)
}

// oneAccord reports the first receipt whose Accord or card is not the first receipt's: accord or
// card, and its index. One journal is one Accord's, so a list of two is refused whole.
func oneAccord(st []receipts.Statement) (string, int) {
	for i, s := range st {
		switch {
		case s.Predicate.Accord != st[0].Predicate.Accord:
			return codeAccord, i
		case s.Predicate.Card != st[0].Predicate.Card:
			return codeCard, i
		}
	}
	return "", -1
}

// verifyCommand is `verify --mediator M --receipts DIR [--founding S]`: receipts.Verify over the
// journal's receipts under the mediator's reference alone. It prints each predicate on standard
// output, and on standard error the count, the Accord, the operator and the module when the founding
// set is given, the counts of verdicts, questions and releases, the claim and the scope line. The
// receipts must all be of one Accord and card, or it is tool error verify, accord or card, at the
// first that differs. A founding set is tool error founding unless it founds the receipts' Accord
// and its mediator is M. A card whose module is not this build's is named after the module line,
// and still verifies (docs/adr/0012-the-kernel-hash.md).
// On a failure it prints the code and the line, and none of these.
func verifyCommand(r *run, args []string) int {
	fs := flags("verify")
	med, dir, founding := fs.String("mediator", "", "the mediator"), fs.String("receipts", "", "journal directory"), fs.String("founding", "", "the founding set")
	if !r.parse(fs, args, "mediator", "receipts") {
		return r.tool(codeUsage)
	}
	list, err := journal.ReadReceipts(*dir, r.stderr)
	if err != nil {
		return r.failed(err)
	}
	st, err := receipts.Verify(*med, list)
	if err != nil {
		return r.notVerified(err)
	}
	if code, i := oneAccord(st); code != "" {
		return r.tool(codeVerify, code, journal.FileReceipts, fmt.Sprintf("line %d", i+1))
	}
	places, err := receipts.Releases(st)
	if err != nil {
		return r.notVerified(err)
	}
	accord, card := "", ""
	if len(st) > 0 {
		accord, card = st[0].Predicate.Accord, st[0].Predicate.Card
	}
	var module []string
	if *founding != "" {
		set, err := os.ReadFile(*founding)
		if err != nil {
			return r.tool(codeFile, "founding")
		}
		set = bytes.TrimSuffix(set, []byte{'\n'})
		at := local.Stamp(r.env.Now())
		if len(st) > 0 {
			at = st[0].Predicate.At
		}
		// The set must be the journal's Accord and name the key the receipts were verified under.
		a, err := setup.Found(set, at)
		if err != nil || a.ID() != accord || a.Mediator() != *med {
			return r.tool(codeFound)
		}
		module = moduleLines(set)
	}
	for _, rc := range list {
		line(r.stdout, predicateOf(rc))
	}
	n := map[string]int{}
	closed, released, blocked := 0, 0, 0
	for i, s := range st {
		n[s.Predicate.Verdict]++
		switch places[i].Role {
		case receipts.RoleClosing:
			closed++
		case receipts.RoleRelease:
			if s.Predicate.Verdict == string(composition.Pass) {
				released++
			} else {
				blocked++
			}
		}
	}
	fmt.Fprintf(r.stderr, "verified %d receipts\n", len(st))
	fmt.Fprintln(r.stderr, "mediator "+*med+" accord "+accord+" card "+card)
	if module == nil {
		fmt.Fprintln(r.stderr, LineNoModule)
	}
	for _, l := range module {
		fmt.Fprintln(r.stderr, l)
	}
	fmt.Fprintf(r.stderr, "%s %d %s %d %s %d\n", WordPass, n[string(composition.Pass)], WordBlock, n[string(composition.Block)], WordAsk, n[string(composition.Escalate)])
	fmt.Fprintf(r.stderr, "questions closed %d open %d\n", closed, n[string(composition.Escalate)]-closed)
	fmt.Fprintf(r.stderr, "releases %d blocked releases %d\n", released, blocked)
	fmt.Fprintln(r.stderr, ClaimVerified)
	fmt.Fprintln(r.stderr, Scope)
	return ExitOK
}

// moduleLines are the lines that name the operator and the decision module of a founded set, and
// the line that names the module when it is not this build's (docs/adr/0012-the-kernel-hash.md).
func moduleLines(set []byte) []string {
	var s struct {
		Mediator struct {
			Operator string `json:"operator"`
			Runs     string `json:"runs"`
		} `json:"mediator"`
		Card struct {
			Kernel struct {
				Version string `json:"version"`
				Hash    string `json:"hash"`
			} `json:"kernel"`
			Checks []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				Hash    string `json:"hash"`
			} `json:"checks"`
		} `json:"card"`
	}
	_ = json.Unmarshal(set, &s)
	out := []string{"operator " + s.Mediator.Operator + " runs " + s.Mediator.Runs,
		"module " + s.Card.Kernel.Version + " " + s.Card.Kernel.Hash}
	if l := otherModule(kernel(s.Card.Kernel)); l != "" {
		out = append(out, l)
	}
	for _, c := range s.Card.Checks {
		out = append(out, "check "+c.Name+" "+c.Version+" "+c.Hash)
	}
	return out
}
