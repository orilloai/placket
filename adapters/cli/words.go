package cli

import (
	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

// The product words: what a person reads for each outcome (product-words-once). The core already
// spells pass, block, ok, the four answers and the reasons, and the export the answer column's own
// words; this file defines only the rest, once.
const (
	WordPass          = string(composition.Pass)
	WordBlock         = string(composition.Block)
	WordAsk           = export.WordAsk
	WordReleased      = "released"
	WordOK            = composition.ReasonOK
	WordAccepted      = string(answer.Accepted)
	WordChanged       = string(answer.Changed)
	WordDeclined      = string(answer.Declined)
	WordCancelled     = string(answer.Cancelled)
	WordTimeout       = receipts.ReasonTimeout
	WordUndeliverable = receipts.ReasonUndeliverable
	WordClosed        = receipts.ReasonClosed
	WordBlocked       = export.AnswerBlocked
	WordOpen          = export.AnswerOpen
)

// The words of reconcile's three lists (receipt-7), each also its member under --json; how a window
// started; a get that handed out a delivery; and the counts verify prints.
const (
	wordRemoved    = "removed"
	wordUnrecorded = "unrecorded"
	wordMissing    = "missing"
	wordFounded    = local.EventFounded
	wordProposed   = "proposed"
	wordReloaded   = local.EventReloaded
	wordClosed     = local.EventClosed
	wordFetched    = "fetched"
	lineQuestions  = "questions closed %d open %d"
	lineReleases   = "releases %d blocked releases %d"
)

// The other lines a person reads.
const (
	LineNotVerified     = "not verified"
	LineReleaseBlocked  = "release blocked"
	LineNotFetched      = "not fetched"
	LineNoDelivery      = "no delivery"
	LineCommentNotCross = "the comment did not cross"
	LineRejected        = "rejected"
	LineError           = "error"
	LineDoor            = "door"
	LineNoModule        = "operator and module not shown: give --founding"
	LineNothing         = "nothing to reconcile"
	LineNoRelease       = "no release matches these bytes"
	LineNoValue         = "no release holds this value at this path"
	LineNoKept          = "no part this key kept matches"
	LineNoKeptValue     = "no part this key kept holds this value at this path"
	// LineTornClose says what to run when a window finds a close cut short (close-1).
	LineTornClose = "the journal holds a close cut short: run placket window close to complete it"
	// LineCloseSize says why a close wrote nothing: its receipt, which lists every delivery not yet
	// fetched, would not fit in one reply of the door (close-2).
	LineCloseSize = "the close would not fit in one reply: once members fetch what is queued for them, run placket window close again"
)

// The window's start-up lines of each judge the card lists, in card order, after its operator line
// (docs/adr/0023-judges.md): the kinds it may be sent and where; that no term lists it, key or no
// key; or that it has no key in the environment variable named, and so sends nothing.
const (
	LineJudgeSends  = "judge %s may send kinds %s to %s, model %s"
	LineJudgeNoTerm = "judge %s sends nothing: no term lists it"
	LineJudgeNoKey  = "judge %s has no key in %s: it sends nothing, and says none"
	// LineAdvised is what an advise judge said of a receipt's part: its name, pass, block or none,
	// and the receipt's id, never its output.
	LineAdvised = "advise %s %s %s"
)

// The lines init prints (docs/adr/0027-placket-init.md): what it founded, then each command for the
// person to run, the command itself on the line after, indented by two spaces.
const (
	LineInitAccord      = "accord %s %s"
	LineInitParty       = "party %s %s %s"
	LineInitWindow      = "start the window of %s, and keep it running:"
	LineInitMCP         = "add placket mcp to Claude Code, serving the agents' labels:"
	LineInitWatch       = "watch for questions, each printed with the lines that answer it:"
	LineInitFetch       = "fetch the applier, at %s:"
	LineInitFetchCommit = "fetch the applier, at %s, pinned to its commit %s:"
	LineInitFetchMain   = "fetch the applier, from main, since this build has no version tag:"
	LineInitApplyFile   = "run the applier, FILE being the file it writes:"
	LineInitApplyMerge  = "run the applier, OWNER/NAME being the repository it merges in:"
)

// LineMisstated is what watch says of a question the window showed that is not the part its ask's
// receipt names: the label and the question's id; it prints nothing else of it.
const LineMisstated = "misstated %s %s: not the part its ask's receipt names, left out"

// The lines verify prints (commands-8, verify-1 to verify-4), each verb filled in order; under
// --json each is also its member.
const (
	LineValid              = "valid: %d receipts signed by mediator %s"
	LineConforms           = "conforms: every decision is one this Accord let its decider make"
	LineConformNoFounding  = "conformance not checked: give --founding"
	LineConformOtherModule = "conformance not checked: the card's module is not this build's"
	// LineConformSealed: conformance ran, but a decision was checked against the terms only where
	// every seal it reads was opened (setup-contract-16, verify-1).
	LineConformSealed = "conforms: every receipt names this Accord, its card and its members, and every answer call given fits; decisions checked against the terms only where the seals they read are opened"
	// The lines of what the sealed terms were checked against (setup-contract-16).
	LineTermsNotChecked = "terms: not checked: the declarations are sealed; with every member's opening, --open checks them"
	LineTermsChecked    = "terms: checked: every seal opens, and every view is the one its member accepted"
	LineSealOpens       = "seal %s: opens: the declaration it seals is its actor's own"
	LineViewMatches     = "view %s: matches its acceptance"
	LineViewNotChecked  = "view %s: not checked: a view reads every member's terms"
	// LineOpenTwice and LineOpenNotMember are the usage lines of an --open that opens no seal of its
	// own: a second opening for one actor, or an opening whose actor is no member of the Accord.
	LineOpenTwice         = "usage: placket verify: two --open files open the seal of one actor"
	LineOpenNotMember     = "usage: placket verify: an --open file opens the seal of an actor that is not a member"
	LineNotConform        = "does not conform: %s at receipts.jsonl line %d"
	LineNotComplete       = "complete: not claimed: this Accord has no close receipt"
	LineSubsetNotComplete = "complete: not claimed: only a subset was given"
	LineComplete          = "complete: against close receipt %s; not claimed: that the mediator signed no other log for this Accord"
	LinePinnedComplete    = "complete: against close receipt %s, which successor accord %s pins; not claimed: that the mediator signed no other log for this Accord"
	LineCustody           = "custody: operator %s is principal of %d of %d members"
	LineSupersedes        = "supersedes accord %s close %s"
	LineChainStart        = "chain %s to %s not checked before seq %d"
	LineWitnessed         = "%s witnessed: signed by key %s, which principal %s bound to role %s"
	LineUnwitnessed       = "%s unwitnessed: only the mediator says a key holding role %s answered"
	LineUnwitnessedCancel = "%s unwitnessed: only the mediator says the asker cancelled"
	LineByJudge           = "%s judge: the mediator says judge %s of the card decided"
	LineByDeadline        = "%s mediator: the deadline passed"
	LineByUndeliverable   = "%s mediator: nobody could be reached to answer"
	LineByClose           = "%s mediator: the Accord was closed"
	LineOutsideSubset     = "%d decisions not checked: their question is not in the subset"
	LineTermsDecided      = "terms decided %d"
	LineUnmatched         = "answer calls matching no receipt %d"
	LineBody              = "body: the part of receipt %s is the one it names"
	// The lines of a member's own view check (verify-2): each receipt of the viewer's against its row,
	// the row in the product's words, and the line of the whole.
	LineViewOfMatches  = "view of %s: receipts.jsonl line %d receipt %s matches %s"
	LineViewOfNotMatch = "view of %s: receipts.jsonl line %d receipt %s does not match %s"
	LineViewRowGive    = "give %s to %s: %s"
	LineViewRowGet     = "get %s from %s: %s"
	LineViewNoRow      = "no row"
	LineViewAll        = "view of %s: every receipt of its own, %d, matches the view it accepted"
	LineViewNotAll     = "view of %s: does not match the view it accepted at receipts.jsonl line %d"
	// LineViewOwnOpening is the usage line of --view beside the viewer's own --open.
	LineViewOwnOpening = "usage: placket verify: --view with the viewer's own --open: the opening checks its terms themselves"
	// LineSuccessorFlags is the usage line of --superseded-by, --successor-accord and
	// --successor-acceptances not given together (close-4, commands-4).
	LineSuccessorFlags = "usage: placket verify: --superseded-by, --successor-accord and --successor-acceptances are given together"
	LineBodyNot        = "body: does not conform at receipts.jsonl line %d"
	// LineContradicted is the decision line of a receipt whose call given does not fit it, which
	// fails row 4 (verify-3); such a receipt is never witnessed or unwitnessed.
	LineContradicted = "%s contradicted: the call given for it does not fit"
	// LineViewNone is the view line when no receipt of the viewer's is in the list.
	LineViewNone = "view of %s: no receipt of its own is in this list, so none was checked against the view it accepted"
)

// The labels of the lines version prints, each also its member under --json (docs/release.md).
const (
	LabelVersion    = "version"
	LabelCommit     = "commit"
	LabelCommitTime = "commitTime"
	LabelGo         = "go"
	LabelModule     = "module"
)

// The claim lines, word for word, one per kind of receipt; every one is followed by Scope
// (claims-stop-at-crossing).
const (
	ClaimPass          = "claim: the mediator judged this part under the Accord's terms and queued it for the receiver."
	ClaimBlock         = "claim: the mediator refused this part for the reason shown. Nothing was queued."
	ClaimAsk           = "claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing."
	ClaimAnswered      = "claim: a key holding the answering role answered. The part named here was judged and queued for the receiver this receipt names."
	ClaimAnswerBlocked = "claim: a key holding the answering role answered. The part named here was refused for the reason shown."
	ClaimNoYes         = "claim: the question closed without a yes. The receipt names the first holder of the answering role."
	ClaimNobody        = "claim: nobody answered. The question closed for the reason shown."
	ClaimClose         = "claim: the mediator closed this Accord. Each chain named here ends at the receipt named; each delivery named was not yet fetched."
	ClaimVerified      = "claim: each receipt is this mediator's signed decision about one part under this Accord's terms."
	// ClaimSigned is verify's claim when it did not check every decision against the terms, or a
	// receipt or a body does not conform (commands-8).
	ClaimSigned        = "claim: each receipt and close is signed by this mediator."
	ClaimReleased      = "claim: these bytes, under the salt they were fetched with, are the part this receipt let cross to this key."
	ClaimReleasedValue = "claim: these bytes are the value at this path inside the part this receipt let cross to this key."
	// ClaimReleasedKept and ClaimReleasedKeptValue are released's claims without --file (commands-7).
	ClaimReleasedKept      = "claim: for each receipt shown, a part this key kept, under the salt it was fetched with, is the part that receipt let cross to this key."
	ClaimReleasedKeptValue = "claim: for each receipt shown, these bytes are the value at this path inside a part this key kept, the part that receipt let cross to this key."
	// ModuleLine names the decision module by the card hash, its one verb.
	ModuleLine = "module: named by card %s."
	// ModuleOther names a card's module that is not this build's, and this build's
	// (docs/adr/0012-the-kernel-hash.md).
	ModuleOther = "module %s %s is not this build's decision module, %s %s"
	// Scope is the scope line.
	Scope = "not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key."
)

// word is the product word for a verdict: pass, block, or ask for escalate.
func word(verdict string) string {
	if verdict == string(composition.Escalate) {
		return WordAsk
	}
	return verdict
}

// claim is the claim line show prints for a receipt, by its kind, verdict and reason.
func claim(p receipts.Predicate) string {
	if p.Kind == receipts.KindCrossing {
		switch p.Verdict {
		case string(composition.Pass):
			return ClaimPass
		case string(composition.Escalate):
			return ClaimAsk
		}
		return ClaimBlock
	}
	switch {
	case p.Verdict == string(composition.Pass):
		return ClaimAnswered
	case p.Reason == receipts.ReasonDeclined || p.Reason == receipts.ReasonCancelled:
		return ClaimNoYes
	case p.Reason == receipts.ReasonTimeout || p.Reason == receipts.ReasonUndeliverable || p.Reason == receipts.ReasonClosed:
		return ClaimNobody
	}
	return ClaimAnswerBlocked
}
