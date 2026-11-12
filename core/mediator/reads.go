package mediator

import (
	"fmt"

	"github.com/orilloai/placket/core/receipts"
)

// Queued is a delivery not yet fetched: the member it is queued for, and its canonical bytes as Get
// would return them (crossing-3, record 0006). Reload takes it back (record 0008). It holds a body
// and a salt, so it prints as a fixed text under every format verb (secrets-never-print).
type Queued struct {
	To       string
	Delivery []byte
}

// Format prints the Queued as a fixed text, whatever the verb.
func (Queued) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("mediator.Queued")) }

// GoString prints the Queued as a fixed text.
func (Queued) GoString() string { return "mediator.Queued" }

// String prints the Queued as a fixed text.
func (Queued) String() string { return "mediator.Queued" }

// Queued returns a copy of the delivery the receipt receiptID queued, and the member it is queued
// for, while it is not yet fetched (crossing-3); false when that receipt queued nothing unfetched.
func (m *Mediator) Queued(receiptID string) (Queued, bool) {
	if !m.founded() {
		return Queued{}, false
	}
	for to, l := range m.queues {
		for _, x := range l {
			if x.id == receiptID {
				return Queued{To: to, Delivery: append([]byte(nil), x.delivery...)}, true
			}
		}
	}
	return Queued{}, false
}

// Result returns, to the question's asker only, the receipts of the closed question questionID: its
// closing receipt, then its release receipt when it has one, as copies. It reads them from the
// receipts with receipts.Releases, the one reader of questions (release-in-resolve), so it answers
// the same after a reload (record 0008). A caller that is not a member is rejected with
// not-a-member (setup-contract-6); another member, an open question, and an id that names no
// question return nil and false.
func (m *Mediator) Result(caller, questionID string) ([]Outcome, bool, error) {
	if !m.founded() {
		return nil, false, failure(CodeUnfounded)
	}
	if !m.accord.Member(caller) {
		return nil, false, &Rejection{Code: NotAMember}
	}
	places, err := receipts.Releases(m.statements)
	if err != nil {
		return nil, false, failure(CodeInternal)
	}
	qi := -1
	for i, s := range m.statements {
		if s.ID == questionID && places[i].Role == receipts.RoleQuestion {
			qi = i
			break
		}
	}
	if qi < 0 || m.statements[qi].Predicate.From != caller {
		return nil, false, nil
	}
	var out []Outcome
	for i := qi + 1; i < len(places); i++ {
		if places[i].Question == qi && (places[i].Role == receipts.RoleClosing || places[i].Role == receipts.RoleRelease) {
			s := m.statements[i]
			out = append(out, Outcome{Verdict: s.Predicate.Verdict, Reason: s.Predicate.Reason, ReceiptID: s.ID,
				Receipt: append([]byte(nil), m.receipts[i]...)})
		}
	}
	if out == nil {
		return nil, false, nil
	}
	return out, true, nil
}
