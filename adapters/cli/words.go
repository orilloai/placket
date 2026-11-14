package cli

import (
	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
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
	WordAccepted      = string(mediator.Accepted)
	WordChanged       = string(mediator.Changed)
	WordDeclined      = string(mediator.Declined)
	WordCancelled     = string(mediator.Cancelled)
	WordTimeout       = receipts.ReasonTimeout
	WordUndeliverable = receipts.ReasonUndeliverable
	WordBlocked       = export.AnswerBlocked
	WordOpen          = export.AnswerOpen
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
	ClaimVerified      = "claim: each receipt is this mediator's signed decision about one part under this Accord's terms."
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
	case p.Reason == receipts.ReasonTimeout || p.Reason == receipts.ReasonUndeliverable:
		return ClaimNobody
	}
	return ClaimAnswerBlocked
}
