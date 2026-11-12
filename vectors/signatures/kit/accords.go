package kit

// The keys every generator names. A key's seed comes from its name (NewKey).
var (
	Mediator       = NewKey("mediator")
	Operator       = NewKey("operator")
	Principal      = NewKey("principal")
	OtherPrincipal = NewKey("other-principal")
	OtherMediator  = NewKey("other-mediator")
	Proposer       = NewKey("proposer")
	Applier        = NewKey("applier")
	Decider        = NewKey("decider")
	Lead           = NewKey("lead")
	Stranger       = NewKey("stranger")
)

// AskAndLog is the ask-and-log Accord of the protocol's scenarios. The proposer may give a proposal
// to the applier, asked about by the decider within 3600 seconds; it gives a note to the applier
// without asking; it gets the decider's answers and changed versions. The applier gets proposals and
// notes from the proposer and changed versions from the decider. The decider gives answers and
// changed versions to the proposer and the applier. No term names the kind tests.
func AskAndLog() Set {
	return Set{
		Mediator: Mediator, Operator: Operator, Principal: Principal,
		Roles: []string{"proposer", "applier", "decider"},
		Parties: []Party{
			{Key: Proposer, Roles: []string{"proposer"}, Terms: []Obj{
				Maybe("give", []string{"applier"}, []string{"proposal"}, []string{"decider"}, 3600),
				Term("want", "give", []string{"applier"}, []string{"note"}),
				Term("want", "get", []string{"decider"}, []string{"answer", "changed-version"}),
			}},
			{Key: Applier, Roles: []string{"applier"}, Terms: []Obj{
				Term("want", "get", []string{"proposer"}, []string{"proposal", "note"}),
				Term("want", "get", []string{"decider"}, []string{"changed-version"}),
			}},
			{Key: Decider, Roles: []string{"decider"}, Terms: []Obj{
				Term("want", "give", []string{"proposer", "applier"}, []string{"answer", "changed-version"}),
			}},
		},
	}
}

// OwnNo is the Accord of the scenario in which the sender refuses: the proposer wants to give a
// draft to the applier and also refuses to give it, and the applier wants to get it, so the only
// refusal is on the sender's side (composition-3).
func OwnNo() Set {
	return Set{
		Mediator: Mediator, Operator: Operator, Principal: Principal,
		Roles: []string{"proposer", "applier"},
		Parties: []Party{
			{Key: Proposer, Roles: []string{"proposer"}, Terms: []Obj{
				Term("want", "give", []string{"applier"}, []string{"draft"}),
				Term("no", "give", []string{"applier"}, []string{"draft"}),
			}},
			{Key: Applier, Roles: []string{"applier"}, Terms: []Obj{
				Term("want", "get", []string{"proposer"}, []string{"draft"}),
			}},
		},
	}
}

// Compose is the Accord of the composition scenarios: one kind per row of section 5, each given by
// the proposer to the applier.
//
//   - brief: the proposer needs to give it and the applier wants to get it (composition-6).
//   - draft: both want it, and the applier also refuses it (composition-3).
//   - memo: both may give and get it, asked about by the decider and the lead within 60 seconds,
//     their roles named in two orders (composition-5).
//   - plan: as memo, but the applier's rule waits 120 seconds (composition-5).
//   - sketch: the proposer wants to give it; the applier has no term for it (composition-2).
//   - audit: the proposer wants to give it; the applier gets it only from an auditor, a role nobody
//     holds (composition-1).
func Compose() Set {
	return Set{
		Mediator: Mediator, Operator: Operator, Principal: Principal,
		Roles: []string{"proposer", "applier", "decider", "lead"},
		Parties: []Party{
			{Key: Proposer, Roles: []string{"proposer"}, Terms: []Obj{
				Term("need", "give", []string{"applier"}, []string{"brief"}),
				Term("want", "give", []string{"applier"}, []string{"draft", "sketch", "audit"}),
				Maybe("give", []string{"applier"}, []string{"memo"}, []string{"decider", "lead"}, 60),
				Maybe("give", []string{"applier"}, []string{"plan"}, []string{"decider", "lead"}, 60),
			}},
			{Key: Applier, Roles: []string{"applier"}, Terms: []Obj{
				Term("want", "get", []string{"proposer"}, []string{"brief", "draft"}),
				Term("no", "get", []string{"proposer"}, []string{"draft"}),
				Maybe("get", []string{"proposer"}, []string{"memo"}, []string{"lead", "decider"}, 60),
				Maybe("get", []string{"proposer"}, []string{"plan"}, []string{"lead", "decider"}, 120),
				Term("want", "get", []string{"auditor"}, []string{"audit"}),
			}},
			{Key: Decider, Roles: []string{"decider"}, Terms: []Obj{
				Term("want", "give", []string{"proposer"}, []string{"answer"}),
			}},
			{Key: Lead, Roles: []string{"lead"}, Terms: []Obj{
				Term("want", "give", []string{"proposer"}, []string{"answer"}),
			}},
		},
	}
}
