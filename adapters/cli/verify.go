package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/conform"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

func init() {
	commands["verify"] = verifyCommand
}

// predicateOf is the predicate member of a receipt's statement, as the receipt holds it.
func predicateOf(receipt []byte) []byte {
	p, err := base64.StdEncoding.DecodeString(memberString(receipt, "payload"))
	if err != nil {
		return nil
	}
	pr, ok := member(p, "predicate")
	if !ok {
		return nil
	}
	return canonicalJSON(pr)
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

// accordOf is the Accord id and the card hash a verified receipt names, a close's included.
func accordOf(v receipts.Verified) (string, string) {
	if c, ok := v.Close(); ok {
		return c.Accord, c.Card
	}
	p := v.Statement().Predicate
	return p.Accord, p.Card
}

// oneAccord reports the first receipt whose Accord or card is not the first receipt's: accord or
// card, and its index. One journal is one Accord's, so a list of two is refused whole.
func oneAccord(list []receipts.Verified) (string, int) {
	if len(list) == 0 {
		return "", -1
	}
	a0, c0 := accordOf(list[0])
	for i, v := range list {
		a, c := accordOf(v)
		switch {
		case a != a0:
			return codeAccord, i
		case c != c0:
			return codeCard, i
		}
	}
	return "", -1
}

// closeOf is the id of the close a verified list holds, its last receipt, or the empty string.
func closeOf(list []receipts.Verified) string {
	if len(list) == 0 {
		return ""
	}
	c, _ := list[len(list)-1].Close()
	return c.ID
}

// firstAt is the time of a list's first receipt, a close's when it is the first, read without
// verifying, or the empty string.
func firstAt(list [][]byte) string {
	if len(list) == 0 {
		return ""
	}
	if s, err := receipts.Decode(list[0]); err == nil {
		return s.Predicate.At
	}
	if c, err := receipts.DecodeClose(list[0]); err == nil {
		return c.At
	}
	return ""
}

// verifyCommand is `verify --receipts DIR (--mediator M | --accord A --founding F) [--founding F]
// [--answers FILE] [--subset] [--delivery FILE] [--superseded-by NEW] [--view V] [--json]` (commands-4,
// commands-8, verify-1 to verify-4, close-4): the journal's receipts checked for validity, under the mediator's reference alone, and,
// with a founding set, for conformance to the Accord, without ever running a judge again. With
// --accord the mediator's reference comes from the founding set whose typed hash is A, the one value
// given out of band; with --mediator, a founding set given must name M and found the receipts'
// Accord. Either is tool error founding before any line prints. The journal's answers.jsonl is read
// when present, with the calls of --answers, to witness answers; --subset checks a subset, whose
// chains may start late; --delivery checks one delivery's body and salt against its receipt, as get
// --json saved it, with or without a founding set.
//
// It prints each predicate on standard output, and on standard error, in order: the Accord's line;
// with a founding set the operator, the module and its judges, each member, custody and what the
// policy supersedes, and without one the line that says they are not shown; the counts of verdicts,
// questions and releases; each chain a subset starts late; one line per receipt the terms did not
// decide, with a founding set; the count of the others and of the calls matching no receipt; and the
// valid, conforms and complete lines, the body line, the claim and the scope line. A validity failure
// prints its code and line and none of these. A log that does not conform, and a body that is not the
// part its receipt names, exit 1 with every line.
//
// The complete line claims completeness only for a whole list that ends in its close (close-4). With
// --superseded-by, the founding set of a successor Accord, founded as founding founds a set, it also
// checks that the successor's supersedes names this Accord and this list's close, and names the
// successor on the complete line; a list with no close, a successor that supersedes nothing, or one
// that names another Accord or close, is tool error verify superseded. It needs --accord and
// --founding.
//
// With --view, one member's own view as it kept it, which needs --founding, it also checks that
// member's receipts against the view it signed (verify-1 rows 0a and 4a, verify-2): after the
// conforms line, one line per receipt of the viewer's and the view line. A view that is not the one
// its viewer accepted is tool error founding acceptance-view; a receipt that does not match exits 1
// with every line; and --view beside the viewer's own --open is a usage error, since the opening
// checks the viewer's terms themselves.
func verifyCommand(r *run, args []string) int {
	fs := flags("verify")
	med, dir, setPath := fs.String("mediator", "", "the mediator"), fs.String("receipts", "", "journal directory"), fs.String("founding", "", "the founding set")
	accordFlag, answersFile := fs.String("accord", "", "the Accord id"), fs.String("answers", "", "a file of answer calls, one a line")
	subset, deliveryFile := fs.Bool("subset", false, "the receipts are a subset"), fs.String("delivery", "", "a delivery, as get --json saved it")
	successor := fs.String("superseded-by", "", "the founding set of the Accord that supersedes this one")
	accFile := fs.String("acceptances", "", "the founding set's acceptances, acceptances.json beside it when not given")
	var opens many
	fs.Var(&opens, flagOpen, "a member's opening, once per opening, for a dispute")
	viewFile := fs.String(flagView, "", "a member's own view, as it kept it, to check its receipts against")
	if !r.parse(fs, args, "receipts") || (*med == "") == (*accordFlag == "") || (*accordFlag != "" && *setPath == "") ||
		(*successor != "" && *accordFlag == "") || ((*accFile != "" || len(opens) > 0 || *viewFile != "") && *setPath == "") {
		return r.tool(codeUsage)
	}
	var viewBytes []byte
	if *viewFile != "" {
		b, err := os.ReadFile(*viewFile)
		if err != nil {
			return r.tool(codeFile, flagView)
		}
		viewBytes = trimLF(b)
	}
	list, err := journal.ReadReceipts(*dir, r.stderr)
	if err != nil {
		return r.failed(err)
	}
	answers, err := journal.ReadAnswers(*dir, r.stderr)
	if err != nil {
		return r.failed(err)
	}
	if *answersFile != "" {
		b, err := os.ReadFile(*answersFile)
		if err != nil {
			return r.tool(codeFile, "answers")
		}
		for _, l := range bytes.Split(b, []byte{'\n'}) {
			if len(l) > 0 {
				answers = append(answers, l)
			}
		}
	}
	var deliveryBytes []byte
	if *deliveryFile != "" {
		b, err := os.ReadFile(*deliveryFile)
		if err != nil {
			return r.tool(codeFile, "delivery")
		}
		deliveryBytes = bytes.TrimSuffix(b, []byte{'\n'})
		if d, ok := member(deliveryBytes, "delivery"); ok {
			deliveryBytes = d
		}
	}
	var set, acceptances []byte
	var openings setup.Openings
	var a *setup.Accord
	if *setPath != "" {
		if set, err = os.ReadFile(*setPath); err != nil {
			return r.tool(codeFile, "founding")
		}
		set = bytes.TrimSuffix(set, []byte{'\n'})
		if *accFile == "" {
			*accFile = filepath.Join(filepath.Dir(*setPath), FileAcceptances)
		}
		if acceptances, err = os.ReadFile(*accFile); err != nil {
			return r.tool(codeFile, "acceptances")
		}
		acceptances = trimLF(acceptances)
		// A verifier holds the openings it is given, each under the actor its declaration names: some
		// members', as in a dispute, or every member's (setup-contract-16).
		if len(opens) > 0 {
			openings = setup.Openings{ByActor: map[string][]byte{}, Some: true}
			for _, p := range opens {
				b, err := os.ReadFile(p)
				if err != nil || openingActor(b) == "" {
					return r.tool(codeFile, flagOpen)
				}
				if _, twice := openings.ByActor[openingActor(b)]; twice {
					fmt.Fprintln(r.stderr, LineOpenTwice)
					return r.tool(codeUsage)
				}
				openings.ByActor[openingActor(b)] = trimLF(b)
			}
			// The viewer's own opening checks its terms themselves: a view beside it checks less.
			if _, own := openings.ByActor[memberString(viewBytes, "viewer")]; viewBytes != nil && own {
				fmt.Fprintln(r.stderr, LineViewOwnOpening)
				return r.tool(codeUsage)
			}
		}
		at := local.Stamp(r.env.Now())
		if t := firstAt(list); t != "" {
			at = t
		}
		id, herr := canonical.TypedHash(set)
		a, err = founding.Found(founding.Inputs{Set: set, Openings: openings, Acceptances: acceptances}, at)
		var se *setup.Error
		if errors.As(err, &se) {
			return r.tool(codeFound, string(se.Code))
		}
		if err != nil || herr != nil || (*accordFlag != "" && id != *accordFlag) || (*med != "" && a.Mediator() != *med) {
			return r.tool(codeFound)
		}
		if *med == "" {
			*med = a.Mediator()
		}
		// An opening whose actor is no member opens no seal of this set, so founding never read it.
		for actor := range openings.ByActor {
			if !a.Member(actor) {
				fmt.Fprintln(r.stderr, LineOpenNotMember)
				return r.tool(codeUsage)
			}
		}
	}

	// Validity (section 8): the verifier's rows, then one Accord and card.
	sub, err := validity(*med, list, *subset)
	if err != nil {
		return r.notVerified(err)
	}
	// A close is in the list as a Verified whose statement is the zero one: it is no decision and no
	// verdict, and rows of its own read it (close-4).
	st := make([]receipts.Statement, len(sub.Verified))
	for i, v := range sub.Verified {
		st[i] = v.Statement()
	}
	if code, i := oneAccord(sub.Verified); code != "" {
		return r.tool(codeVerify, code, journal.FileReceipts, fmt.Sprintf("line %d", i+1))
	}
	accord, card := *accordFlag, ""
	if len(st) > 0 {
		accord, card = accordOf(sub.Verified[0])
	}
	if a != nil && len(st) > 0 && a.ID() != accord {
		return r.tool(codeFound)
	}
	if a != nil {
		accord = a.ID()
	}
	closeID := closeOf(sub.Verified)
	pinned := ""
	if *successor != "" {
		b, err := os.ReadFile(*successor)
		if err != nil {
			return r.tool(codeFile, "superseded-by")
		}
		b = bytes.TrimSuffix(b, []byte{'\n'})
		next, err := setup.Found(b, setup.Openings{}, local.Stamp(r.env.Now()))
		if err != nil {
			return r.tool(codeFound)
		}
		acc, cl, ok := next.Supersedes()
		if !ok || closeID == "" || acc != accord || cl != closeID {
			return r.tool(codeVerify, detailSuperseded)
		}
		pinned = next.ID()
	}

	// Conformance (verify-2): rows 1 to 4, then the body, row 5.
	e := conform.Evidence{Accord: accord, Mediator: *med, Founding: set, Acceptances: acceptances, Openings: openings,
		Receipts: list, Answers: answers, Subset: *subset, Kernel: composition.KernelHash(), View: viewBytes}
	report, cerr := conform.Check(e)
	var conformErr *conform.Error
	if cerr != nil && !errors.As(cerr, &conformErr) {
		return r.notVerified(cerr)
	}
	if conformErr != nil && conformErr.Code == conform.CodeAcceptanceView {
		// Row 0a: the view is not the one its viewer accepted, a founding fact (verify-1).
		return r.tool(codeFound, string(conformErr.Code))
	}
	bodyLine := ""
	failed := conformErr != nil
	if deliveryBytes != nil && conformErr == nil {
		if err := conform.Body(report, deliveryBytes); err != nil {
			var be *conform.Error
			errors.As(err, &be)
			if be.Index < 0 {
				// A delivery that is not one, or whose receipt is none of these, has no line to name.
				return r.tool(codeDelivery)
			}
			bodyLine = fmt.Sprintf(LineBodyNot, be.Index+1)
			if a != nil {
				conformErr = be
			}
			failed = true
		} else {
			bodyLine = fmt.Sprintf(LineBody, field(memberString(deliveryBytes, "receiptId")))
		}
	}

	var module []string
	var members []any
	if a != nil {
		module = moduleLines(set)
		for _, m := range a.Members() {
			module = append(module, "member "+m+" roles "+strings.Join(a.Roles(m), ",")+" principal "+a.Principal(m))
			members = append(members, map[string]any{"actor": m, "roles": a.Roles(m), "principal": a.Principal(m)})
		}
		of := 0
		for _, m := range a.Members() {
			if a.Principal(m) == a.Operator() {
				of++
			}
		}
		custody := fmt.Sprintf(LineCustody, a.Operator(), of, len(a.Members()))
		module = append(module, custody)
		r.put("custody", custody)
		if acc, cl, ok := a.Supersedes(); ok {
			module = append(module, fmt.Sprintf(LineSupersedes, acc, cl))
			r.put("supersedes", map[string]string{"accord": acc, "close": cl})
		}
		r.putModule(set)
		r.put("members", members)
		module = append(module, r.termsLines(a)...)
	}
	predicates := []any{}
	for _, rc := range list {
		p := predicateOf(rc)
		line(r.stdout, p)
		predicates = append(predicates, json.RawMessage(p))
	}
	n := map[string]int{}
	closed, released, blocked := 0, 0, 0
	for i, s := range st {
		if s.ID == "" {
			continue
		}
		n[s.Predicate.Verdict]++
		switch sub.Places[i].Role {
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
	fmt.Fprintln(r.stderr, "mediator "+*med+" accord "+accord+" card "+card)
	if module == nil {
		fmt.Fprintln(r.stderr, LineNoModule)
	}
	for _, l := range module {
		fmt.Fprintln(r.stderr, l)
	}
	fmt.Fprintf(r.stderr, "%s %d %s %d %s %d\n", WordPass, n[string(composition.Pass)], WordBlock, n[string(composition.Block)], WordAsk, n[string(composition.Escalate)])
	fmt.Fprintf(r.stderr, lineQuestions+"\n", closed, n[string(composition.Escalate)]-closed)
	fmt.Fprintf(r.stderr, lineReleases+"\n", released, blocked)
	starts := []any{}
	for _, s := range sub.Starts {
		fmt.Fprintf(r.stderr, LineChainStart+"\n", s.From, s.To, s.Seq)
		starts = append(starts, map[string]any{"from": s.From, "to": s.To, "seq": s.Seq})
	}
	decisions := []any{}
	terms, outside := 0, 0
	// The closing of each question, by the question's index: an unwitnessed release names the roles
	// of its closing's from, the answerer, not of its own, the asker.
	closings := map[int]receipts.Statement{}
	for i, pl := range sub.Places {
		if pl.Role == receipts.RoleClosing {
			closings[pl.Question] = st[i]
		}
	}
	for _, d := range report.Decisions {
		switch {
		case d.By.Decider == receipts.ByTerms:
			terms++
		case !d.Checked && *subset:
			outside++
		case a != nil:
			answerer := st[d.Index].Predicate.From
			if pl := sub.Places[d.Index]; pl.Role == receipts.RoleRelease {
				answerer = closings[pl.Question].Predicate.From
			}
			l := decisionLine(a, st[d.Index], d, answers, answerer)
			fmt.Fprintln(r.stderr, l)
			decisions = append(decisions, map[string]any{memberReceiptID: st[d.Index].ID, "line": l})
		}
	}
	if *subset {
		fmt.Fprintf(r.stderr, LineOutsideSubset+"\n", outside)
	}
	fmt.Fprintf(r.stderr, LineTermsDecided+"\n", terms)
	if report.Unmatched > 0 {
		fmt.Fprintf(r.stderr, LineUnmatched+"\n", report.Unmatched)
	}
	valid := fmt.Sprintf(LineValid, len(st), *med)
	conforms := LineConforms
	switch {
	case conformErr != nil:
		conforms = fmt.Sprintf(LineNotConform, conformErr.Code, conformErr.Index+1)
	case a == nil:
		conforms = LineConformNoFounding
	case !report.Module:
		conforms = LineConformOtherModule
	case !report.Terms:
		conforms = LineConformSealed
	}
	complete := LineNotComplete
	switch {
	case *subset:
		complete = LineSubsetNotComplete
	case pinned != "":
		complete = fmt.Sprintf(LinePinnedComplete, closeID, pinned)
	case closeID != "":
		complete = fmt.Sprintf(LineComplete, closeID)
	}
	fmt.Fprintln(r.stderr, valid)
	fmt.Fprintln(r.stderr, conforms)
	if viewBytes != nil {
		r.viewLines(report, st)
	}
	fmt.Fprintln(r.stderr, complete)
	if bodyLine != "" {
		fmt.Fprintln(r.stderr, bodyLine)
		r.put("body", bodyLine)
	}
	fmt.Fprintln(r.stderr, ClaimVerified)
	fmt.Fprintln(r.stderr, Scope)
	for name, v := range map[string]any{
		"verified": len(st), "mediator": *med, memberAccord: accord, memberCard: card,
		"counts":          map[string]int{WordPass: n[string(composition.Pass)], WordBlock: n[string(composition.Block)], WordAsk: n[string(composition.Escalate)]},
		"questionsClosed": closed, "questionsOpen": n[string(composition.Escalate)] - closed, "releases": released, "blockedReleases": blocked,
		"predicates": predicates, memberClaim: ClaimVerified, memberNotClaimed: Scope,
		"valid": valid, "conforms": conforms, "complete": complete, "decisions": decisions, "starts": starts, "unmatched": report.Unmatched,
	} {
		r.put(name, v)
	}
	if failed {
		return ExitBlock
	}
	return ExitOK
}

// validity runs the verifier over the list, or over a subset, and returns what it verified.
func validity(mediatorRef string, list [][]byte, subset bool) (receipts.Subset, error) {
	if subset {
		return receipts.VerifySubset(mediatorRef, list)
	}
	v, err := receipts.Verify(mediatorRef, list)
	if err != nil {
		return receipts.Subset{}, err
	}
	places, err := receipts.Releases(v)
	if err != nil {
		return receipts.Subset{}, err
	}
	return receipts.Subset{Verified: v, Places: places}, nil
}

// decisionLine is the line verify prints for a receipt the terms did not decide (commands-8): an
// answer witnessed, by the key that signed its call and who bound it to which roles, or unwitnessed;
// a judge of the card; or the mediator, by its event. An unwitnessed answer names the roles of the
// answerer, the from of the question's closing, as its binding gives them. A call is read only in
// its canonical bytes, as conform matches it.
func decisionLine(a *setup.Accord, s receipts.Statement, d conform.Decision, answers [][]byte, answerer string) string {
	switch d.By.Decider {
	case receipts.ByJudge:
		return fmt.Sprintf(LineByJudge, s.ID, d.By.Judge)
	case receipts.ByMediator:
		switch d.By.Event {
		case receipts.EventDeadline:
			return fmt.Sprintf(LineByDeadline, s.ID)
		case receipts.EventClose:
			return fmt.Sprintf(LineByClose, s.ID)
		}
		return fmt.Sprintf(LineByUndeliverable, s.ID)
	}
	if d.Witness == conform.Witnessed {
		for _, b := range answers {
			if c, err := canonical.Form(b); err != nil || !bytes.Equal(c, b) || answer.ID(b) != d.By.Answer {
				continue
			}
			if c, err := answer.Read(b); err == nil {
				return fmt.Sprintf(LineWitnessed, s.ID, c.From, a.Principal(c.From), strings.Join(a.Roles(c.From), ","))
			}
		}
	}
	if s.Predicate.Reason == receipts.ReasonCancelled {
		return fmt.Sprintf(LineUnwitnessedCancel, s.ID)
	}
	return fmt.Sprintf(LineUnwitnessed, s.ID, strings.Join(a.Roles(answerer), ","))
}

// putModule puts the operator and the decision module of a founded set: operator, runs, module and
// judges, the card's judges as it lists them.
func (r *run) putModule(set []byte) {
	judges := []json.RawMessage{}
	if c, ok := member(set, "card", "judges"); ok {
		var list []json.RawMessage
		_ = json.Unmarshal(c, &list)
		judges = append(judges, list...)
	}
	k, _ := member(set, "card", "kernel")
	r.put(memberOperator, memberString(set, "mediator", "operator"))
	r.put("runs", memberString(set, "mediator", "runs"))
	r.put(LabelModule, k)
	r.put("judges", judges)
}

// moduleLines are the lines that name the operator and the decision module of a founded set, the
// line that names the module when it is not this build's (docs/adr/0012-the-kernel-hash.md), and one
// line per judge of the card, in card order.
func moduleLines(set []byte) []string {
	k, _ := kernelOf(cardOf(set))
	out := []string{"operator " + memberString(set, "mediator", "operator") + " runs " + memberString(set, "mediator", "runs"),
		LabelModule + " " + k.Version + " " + k.Hash}
	if l := otherModule(k); l != "" {
		out = append(out, l)
	}
	var judges []json.RawMessage
	if c, ok := member(set, "card", "judges"); ok {
		_ = json.Unmarshal(c, &judges)
	}
	for _, b := range judges {
		var c struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Hash    string `json:"hash"`
		}
		_ = strict.Decode(b, &c)
		out = append(out, "judge "+c.Name+" "+c.Version+" "+c.Hash)
	}
	return out
}

// FileAcceptances is the name of the acceptances' file, kept beside the founding set
// (setup-contract-15).
const FileAcceptances = "acceptances.json"

// termsLines are verify's lines of what the terms behind the sealed set were checked against
// (setup-contract-16): one per seal an opening given opens; with every member opened, one per member
// whose view is the one it accepted, which founding has checked, and the line that the terms were
// checked; otherwise one per opened member whose view could not be built, and the line that the
// terms were not checked. It puts them under terms, seals and views.
func (r *run) termsLines(a *setup.Accord) []string {
	var out []string
	seals, views := []string{}, []string{}
	for _, m := range a.Members() {
		if a.Opened(m) {
			l := fmt.Sprintf(LineSealOpens, m)
			seals = append(seals, l)
			out = append(out, l)
		}
	}
	terms := LineTermsNotChecked
	for _, m := range a.Members() {
		switch {
		case a.TermsChecked():
			views = append(views, fmt.Sprintf(LineViewMatches, m))
		case a.Opened(m):
			views = append(views, fmt.Sprintf(LineViewNotChecked, m))
		}
	}
	if a.TermsChecked() {
		terms = LineTermsChecked
	}
	out = append(append(out, views...), terms)
	r.put("seals", seals)
	r.put("views", views)
	r.put("terms", terms)
	return out
}

// flagOpen is verify's flag that gives a member's opening, for a dispute (setup-contract-16). It is
// spelled as the product word for a question still open, which it shares and is defined once as.
const flagOpen = WordOpen

// flagView is verify's flag that gives one member's own view, as it kept it (verify-1 rows 0a and
// 4a).
const flagView = "view"

// viewLines prints verify's view lines (commands-8, verify-2): one per receipt of the viewer's read
// against its view, in list order, with its line of receipts.jsonl, its id and its row in the
// product's words, or that the view has no row for it, and whether it matches; then the view line,
// that every one matches the view the viewer accepted, or the line of the first that does not. It
// puts them under viewer, viewReceipts and viewCheck.
func (r *run) viewLines(report conform.Report, st []receipts.Statement) {
	viewer := field(report.Viewer)
	lines := []any{}
	first := -1
	for _, v := range report.Viewed {
		what := LineViewNoRow
		if v.Outcome != "" {
			what = viewWord(v.Outcome)
		}
		row := fmt.Sprintf(LineViewRowGive, field(v.Kind), field(v.Counterpart), what)
		if v.Dir == "get" {
			row = fmt.Sprintf(LineViewRowGet, field(v.Kind), field(v.Counterpart), what)
		}
		f := LineViewOfMatches
		if !v.Matches {
			f = LineViewOfNotMatch
			if first < 0 {
				first = v.Index
			}
		}
		l := fmt.Sprintf(f, viewer, v.Index+1, st[v.Index].ID, row)
		fmt.Fprintln(r.stderr, l)
		lines = append(lines, map[string]any{memberReceiptID: st[v.Index].ID, "line": l})
	}
	check := fmt.Sprintf(LineViewAll, viewer, len(report.Viewed))
	if first >= 0 {
		check = fmt.Sprintf(LineViewNotAll, viewer, first+1)
	}
	fmt.Fprintln(r.stderr, check)
	r.put("viewer", report.Viewer)
	r.put("viewReceipts", lines)
	r.put("viewCheck", check)
}

// viewWord is the product word of a view row's outcome, as view prints it: pass, block or ask.
func viewWord(outcome string) string {
	switch outcome {
	case founding.Crosses:
		return WordPass
	case founding.Asked:
		return WordAsk
	}
	return WordBlock
}
