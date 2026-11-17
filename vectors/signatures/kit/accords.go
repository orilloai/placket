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

// LongWait is the ask-and-log Accord with the largest timeout the term format holds, 2^53 - 1
// seconds, so that every deadline is past the last time a timestamp can write (maybe-1).
func LongWait() Set {
	s := AskAndLog()
	s.Parties[0].Terms[0] = Maybe("give", []string{"applier"}, []string{"proposal"}, []string{"decider"}, 9007199254740991)
	return s
}

// Unanswered is the ask-and-log Accord whose proposals are asked about by auditor, a role no binding
// gives and the policy does not list, which founding accepts (setup-contract-5): its questions have
// no answerer (maybe-4).
func Unanswered() Set {
	s := AskAndLog()
	s.Parties[0].Terms[0] = Maybe("give", []string{"applier"}, []string{"proposal"}, []string{"auditor"}, 3600)
	return s
}

// AnswerUnderMaybe is the ask-and-log Accord in which the proposer gets answers from the decider
// under a maybe term of its own, asked about by the decider within 60 seconds, so an accepted answer
// meets a maybe term (composition-5).
func AnswerUnderMaybe() Set {
	s := AskAndLog()
	s.Parties[0].Terms[2] = Maybe("get", []string{"decider"}, []string{"answer"}, []string{"decider"}, 60)
	return s
}

// Judged is the Accord of the judge scenarios (composition-7), under JudgedCard, whose one judge is
// jev. The proposer gives each kind to the applier, asked about by the decider within 3600 seconds;
// the kinds differ only in the judges the two sides' maybe terms list.
//
//   - report: both sides list jev with use decide, so jev's judgment decides.
//   - review: only the proposer's term lists jev, so the part is asked.
//   - survey: the proposer lists jev to decide and the applier to advise, so the part is asked.
//   - digest: both sides list jev to advise, so the part is asked.
//   - ledger: as report, and the proposer also wants to give it, so the part is asked.
//
// The decider gives answers and changed versions to the proposer, who gets them.
func Judged() Set {
	decide, advise := Judges("jev", "decide"), Judges("jev", "advise")
	to := []string{"decider"}
	return Set{
		Mediator: Mediator, Operator: Operator, Principal: Principal, Card: JudgedCard(),
		Roles: []string{"proposer", "applier", "decider"},
		Parties: []Party{
			{Key: Proposer, Roles: []string{"proposer"}, Terms: []Obj{
				JudgedMaybe("give", []string{"applier"}, []string{"report", "review", "survey", "ledger"}, to, 3600, decide),
				JudgedMaybe("give", []string{"applier"}, []string{"digest"}, to, 3600, advise),
				Term("want", "give", []string{"applier"}, []string{"ledger"}),
				Term("want", "get", []string{"decider"}, []string{"answer", "changed-version"}),
			}},
			{Key: Applier, Roles: []string{"applier"}, Terms: []Obj{
				JudgedMaybe("get", []string{"proposer"}, []string{"report", "ledger"}, to, 3600, decide),
				Maybe("get", []string{"proposer"}, []string{"review"}, to, 3600),
				JudgedMaybe("get", []string{"proposer"}, []string{"survey", "digest"}, to, 3600, advise),
			}},
			{Key: Decider, Roles: []string{"decider"}, Terms: []Obj{
				Term("want", "give", []string{"proposer", "applier"}, []string{"answer", "changed-version"}),
			}},
		},
	}
}

// Superseding is the ask-and-log Accord whose policy supersedes an earlier Accord, by its id and the
// id of its close receipt (setup-contract-11).
func Superseding() Set {
	s := AskAndLog()
	s.Supersedes = Obj{"accord": HashOf("placket vectors: an earlier Accord"), "close": HashOf("placket vectors: its close receipt")}
	return s
}
