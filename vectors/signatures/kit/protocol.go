package kit

import "sort"

// Times every generator uses: the founding time and the bindings' end.
const (
	FoundedAt = "2026-11-12T08:00:00Z"
	Until     = "2027-11-12T00:00:00Z"
)

// Card is the card of every founding set: spec 0.1, a kernel named by its version and the hash of
// its name, and no judges (setup-contract-4).
func Card() Obj {
	return Obj{
		"spec":   Spec,
		"kernel": Obj{"version": "0.1.0", "hash": KernelHash},
		"judges": []any{},
	}
}

// KernelHash is the kernel hash of Card's kernel: the hash of its name. A verifier the conform
// vectors run trusts it to decide again (verify-2).
var KernelHash = HashOf("placket reference decision module 0.1")

// Jev is the judge JudgedCard lists, as the card pins it: its name, version and hash.
var Jev = Obj{"name": "jev", "version": "1.0.0", "hash": HashOf("placket vectors judge jev 1.0.0")}

// JudgedCard is Card with one judge, jev, in its judges (setup-contract-4).
func JudgedCard() Obj {
	c := Card()
	c["judges"] = []any{Jev}
	return c
}

// Judges builds a maybe term's resolve.judges from name and use pairs, in order
// (setup-contract-1).
func Judges(pairs ...string) []any {
	var out []any
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Obj{"name": pairs[i], "use": pairs[i+1]})
	}
	return out
}

// JudgedMaybe is a maybe term with its rule and the judges its resolve lists.
func JudgedMaybe(dir string, with, kinds, to []string, timeout int64, judges []any) Obj {
	t := Maybe(dir, with, kinds, to, timeout)
	t["resolve"].(Obj)["judges"] = judges
	return t
}

// Term builds a term: a stance, a direction, the roles it names, the kinds it selects, and for a
// maybe term its resolve (setup-contract-1).
func Term(stance, dir string, with, kinds []string) Obj {
	return Obj{"stance": stance, "dir": dir, "with": selectors(with), "select": Obj{"kind": strs(kinds)}}
}

// Maybe builds a maybe term with its rule.
func Maybe(dir string, with, kinds, to []string, timeout int64) Obj {
	t := Term("maybe", dir, with, kinds)
	t["resolve"] = Obj{"to": selectors(to), "timeout": timeout}
	return t
}

func selectors(roles []string) []any {
	out := make([]any, len(roles))
	for i, r := range roles {
		out[i] = "role:" + r
	}
	return out
}

func strs(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}

// Declaration builds an actor's declaration of its terms, signed by the actor.
func Declaration(actor Key, terms ...Obj) Obj {
	l := make([]any, len(terms))
	for i, t := range terms {
		l[i] = t
	}
	return Signed(Obj{"actor": actor.Ref, "terms": l}, TypeDeclaration, actor)
}

// Binding builds a principal's binding of an actor to roles under a mediator and the policy whose
// typed hash it names, until a time (setup-contract-2).
func Binding(actor, principal, mediator Key, policy, until string, roles ...string) Obj {
	return Signed(Obj{
		"actor": actor.Ref, "principal": principal.Ref, "roles": strs(roles), "policy": policy,
		"mediator": mediator.Ref, "until": until,
	}, TypeBinding, principal)
}

// A Grant is an entry of a policy: a principal and the roles it may grant (setup-contract-9).
type Grant struct {
	Principal Key
	Roles     []string
}

// Policy builds a policy: the roles that must be held, and one entry per grant, in the order given.
func Policy(roles []string, grants ...Grant) Obj {
	ps := make([]any, len(grants))
	for i, g := range grants {
		ps[i] = Obj{"principal": g.Principal.Ref, "roles": strs(g.Roles)}
	}
	return Obj{"roles": strs(roles), "principals": ps}
}

// A Party is a member of a set: its key, the roles its binding gives it, its terms, and the principal
// that signs its binding, the set's Principal when By is the zero Key.
type Party struct {
	Key   Key
	Roles []string
	Terms []Obj
	By    Key
}

// A Set is a founding set as a generator describes it. Grants is the policy's entries; when it is
// empty, the policy has one entry, the set's Principal, which may grant every role of Roles and of
// every party's binding.
type Set struct {
	Mediator, Operator, Principal Key
	Roles                         []string
	Grants                        []Grant
	Parties                       []Party
	// Card is the set's card; nil is Card's.
	Card Obj
	// Supersedes is the policy's supersedes, {accord, close}, or nil (setup-contract-11).
	Supersedes Obj
}

// Policy writes the set's policy (setup-contract-9, setup-contract-11).
func (s Set) Policy() Obj {
	p := s.policy()
	if s.Supersedes != nil {
		p["supersedes"] = s.Supersedes
	}
	return p
}

func (s Set) policy() Obj {
	if len(s.Grants) > 0 {
		return Policy(s.Roles, s.Grants...)
	}
	all := append([]string(nil), s.Roles...)
	for _, p := range s.Parties {
		for _, r := range p.Roles {
			if !contains(all, r) {
				all = append(all, r)
			}
		}
	}
	return Policy(s.Roles, Grant{Principal: s.Principal, Roles: all})
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// Build writes the set: the policy, the mediator, the card, and each party's declaration and binding,
// signed by its principal for the set's policy, both lists in actor-reference order
// (setup-contract-9, setup-contract-10).
func (s Set) Build() Obj {
	policy := s.Policy()
	var decls, binds []Obj
	for _, p := range s.Parties {
		by := p.By
		if by.Ref == "" {
			by = s.Principal
		}
		decls = append(decls, Declaration(p.Key, p.Terms...))
		binds = append(binds, Binding(p.Key, by, s.Mediator, HashOf(policy), Until, p.Roles...))
	}
	card := s.Card
	if card == nil {
		card = Card()
	}
	return FoundingSet(policy, Obj{"actor": s.Mediator.Ref, "operator": s.Operator.Ref, "runs": "process"}, card, decls, binds)
}

// FoundingSet assembles a set from its parts, sorting the declarations and the bindings by actor.
func FoundingSet(policy, mediator, card Obj, decls, binds []Obj) Obj {
	SortByActor(decls)
	SortByActor(binds)
	d := make([]any, len(decls))
	for i, x := range decls {
		d[i] = x
	}
	b := make([]any, len(binds))
	for i, x := range binds {
		b[i] = x
	}
	return Obj{"policy": policy, "mediator": mediator, "card": card, "declarations": d, "bindings": b}
}

// SortByActor sorts objects by their actor member, in actor-reference order.
func SortByActor(l []Obj) {
	sort.SliceStable(l, func(i, j int) bool { return Less16(l[i]["actor"].(string), l[j]["actor"].(string)) })
}

// TextPart builds a part with a text body, its kind and its salt.
func TextPart(kind, salt, text string) Obj { return part(kind, salt, "text", text) }

// RawPart builds a part with a raw body, given as bytes and written in standard base64.
func RawPart(kind, salt string, raw []byte) Obj { return part(kind, salt, "raw", B64(raw)) }

// DataPart builds a part with a data body.
func DataPart(kind, salt string, data any) Obj { return part(kind, salt, "data", data) }

func part(kind, salt, body string, value any) Obj {
	return Obj{body: value, "metadata": Obj{Extension: Obj{"kind": kind, "salt": salt}}}
}

// Message builds a crossing message of one part, given by a member in ROLE_USER, to a receiver
// under an Accord (crossing-1).
func Message(id, accord string, to Key, p Obj) Obj {
	return Obj{
		"messageId":  id,
		"role":       "ROLE_USER",
		"parts":      []any{p},
		"extensions": []any{Extension},
		"metadata":   Obj{Extension: Obj{"accord": accord, "to": to.Ref}},
	}
}

// Ext returns a message's crossing extension object.
func Ext(m Obj) Obj { return m["metadata"].(Obj)[Extension].(Obj) }

// PartOf returns a message's first part.
func PartOf(m Obj) Obj { return m["parts"].([]any)[0].(Obj) }

// Envelope builds the sender's envelope over message bytes at a sequence number, signed by the
// sender (crossing-4).
func Envelope(seq int64, message []byte, sender Key) Obj {
	return Signed(Obj{"seq": seq, "hash": Hash(message)}, TypeEnvelope, sender)
}

// Body returns a part's body bytes: the UTF-8 of text, the decoded raw, or the canonical bytes of
// data (crossing-2), and the name of its body member.
func Body(p Obj) ([]byte, string) {
	switch {
	case p["text"] != nil:
		return []byte(p["text"].(string)), "text"
	case p["raw"] != nil:
		return FromB64(p["raw"].(string)), "raw"
	default:
		return Canon(p["data"]), "data"
	}
}

// Digest is a part's salted digest: the SHA-256 of its salt's bytes and its body's bytes
// (crossing-3).
func Digest(p Obj) string {
	meta := p["metadata"].(Obj)[Extension].(Obj)
	body, _ := Body(p)
	return Sum(FromHex(meta["salt"].(string)), body)
}

// A Pred is what a receipt states, as a generator gives it (receipt-3).
type Pred struct {
	Accord, Card    string
	Kind            string
	From, To        string
	Seq             int64
	Prev            string
	At              string
	Sender          string
	ESeq            int64
	EHash           string
	Verdict, Reason string
	// By is the predicate's decider (receipt-11): "terms", or an object of one member; nil leaves it
	// out, for a vector of a predicate without one.
	By any
	// Mediator is the predicate's mediator; empty means the signing key's.
	Mediator string
}

// The deciders of receipt-11, as a predicate's by holds them.
var (
	ByTerms         any = "terms"
	ByDeadline          = Obj{"mediator": "deadline"}
	ByUndeliverable     = Obj{"mediator": "undeliverable"}
)

// ByJudge is the by of a receipt a judge of the card decided, by its name.
func ByJudge(name string) Obj { return Obj{"judge": name} }

// ByAnswer is the by of a receipt an answer decided: the typed hash of its call (canonical-3).
func ByAnswer(call Obj) Obj { return Obj{"answer": HashOf(call)} }

// Statement writes a receipt's statement (receipt-1, receipt-2, receipt-3).
func Statement(name, digest string, p Pred, mediator Key) Obj {
	med := p.Mediator
	if med == "" {
		med = mediator.Ref
	}
	pred := Obj{
		"accord": p.Accord, "card": p.Card, "mediator": med, "kind": p.Kind,
		"from": p.From, "to": p.To, "seq": p.Seq, "at": p.At,
		"envelope": Obj{"sender": p.Sender, "seq": p.ESeq, "hash": p.EHash},
		"verdict":  p.Verdict, "reason": p.Reason,
	}
	if p.By != nil {
		pred["by"] = p.By
	}
	if p.Prev != "" {
		pred["prev"] = p.Prev
	}
	return Obj{
		"_type":         StatementType,
		"subject":       []any{Obj{"name": name, "digest": Obj{"salted-sha256": digest}}},
		"predicateType": PredicateType,
		"predicate":     pred,
	}
}

// Receipt signs statement bytes as a receipt under the key (receipt-1) and returns it and its id,
// the typed hash of the statement's bytes.
func Receipt(statement []byte, signer Key) (Obj, string) {
	sig := signer.Sign(PAE(PayloadType, statement))
	return Obj{
		"payloadType": PayloadType,
		"payload":     B64(statement),
		"signatures":  []any{Obj{"keyid": signer.Ref, "sig": B64(sig)}},
	}, Hash(statement)
}

// A Scene follows one mediator's state through a scenario as the protocol text says it changes: the
// chain of each pair, each receiver's queue, the open questions and every receipt written.
type Scene struct {
	Accord, Card string
	Mediator     Key
	Keys         map[string]Key
	// FoundedAt is the time the scenario founds its set at; NewScene sets it to the kit's.
	FoundedAt string
	chains    map[[2]string]link
	written   []any
	queues    map[string][]any
	pending   []any
	held      map[string]heldPart
	steps     []any
}

// A heldPart is what a question holds (maybe-1): its asker and receiver, the echoed envelope, the
// asked part's kind and digest, and the part as its delivery would carry it.
type heldPart struct {
	asker, receiver Key
	eseq            int64
	ehash           string
	kind, digest    string
	delivery        Obj
	receipt         Obj
}

// Asked returns the ask's receipt of the open question q (maybe-1).
func (s *Scene) Asked(q string) Obj { return s.held[q].receipt }

// AnswerCall builds an answer call (maybe-11): the Accord, the question, the action and, for an
// accept or a change, the salted digest of the answer and whether it asks for release, from the
// signer, signed under TypeAnswer.
func AnswerCall(accord, question, action, digest string, release bool, signer Key) Obj {
	c := Obj{"accord": accord, "question": question, "action": action, "from": signer.Ref}
	if action == "accepted" || action == "changed" {
		c["digest"], c["release"] = digest, release
	}
	return Signed(c, TypeAnswer, signer)
}

// Call is the answer call of the signer on the question q of the scene's Accord, for an accept or a
// change over text under salt.
func (s *Scene) Call(q, action, text, salt string, release bool, signer Key) Obj {
	digest := ""
	if action == "accepted" || action == "changed" {
		digest = Sum(FromHex(salt), []byte(text))
	}
	return AnswerCall(s.Accord, q, action, digest, release, signer)
}

type link struct {
	seq int64
	id  string
}

// NewScene starts a scene of a founded set, run by the mediator's key, with every key a step names.
func NewScene(set Obj, mediator Key, keys ...Key) *Scene {
	s := &Scene{
		Accord: HashOf(set), Card: HashOf(set["card"]), Mediator: mediator,
		Keys: map[string]Key{}, chains: map[[2]string]link{}, queues: map[string][]any{},
		held: map[string]heldPart{}, FoundedAt: FoundedAt,
	}
	for _, k := range keys {
		s.Keys[k.Name] = k
	}
	s.Keys[mediator.Name] = mediator
	return s
}

// Write writes the next receipt of the pair from, to: its seq one above the pair's last and its prev
// that receipt's id (receipt-5), with its decider by (receipt-11).
func (s *Scene) Write(kind string, from, to Key, name, digest, at string, sender Key, eseq int64, ehash, verdict, reason string, by any) (Obj, string) {
	pair := [2]string{from.Ref, to.Ref}
	last := s.chains[pair]
	st := Statement(name, digest, Pred{
		Accord: s.Accord, Card: s.Card, Kind: kind, From: from.Ref, To: to.Ref,
		Seq: last.seq + 1, Prev: last.id, At: at,
		Sender: sender.Ref, ESeq: eseq, EHash: ehash, Verdict: verdict, Reason: reason, By: by,
	}, s.Mediator)
	r, id := Receipt(Canon(st), s.Mediator)
	s.chains[pair] = link{seq: last.seq + 1, id: id}
	s.written = append(s.written, Obj{"receiptId": id, "receipt": r})
	return r, id
}

// Give adds a give step from the sender at seq whose verdict and reason the generator derived by
// hand, the terms its decider, writes its receipt, and queues the part for its receiver on a pass or
// holds it as a question on an escalate (maybe-1, crossing-3). It returns the receipt's id.
func (s *Scene) Give(at string, from Key, seq int64, message Obj, to Key, verdict, reason string) string {
	return s.GiveJudged(at, from, seq, message, to, nil, verdict, reason, ByTerms)
}

// Judgment is a give step's judge: the judgment handed to the decision, a judge's card name and its
// verdict, pass or block (conformance-vectors-3, composition-7).
func Judgment(name, verdict string) Obj { return Obj{"name": name, "verdict": verdict} }

// GiveJudged adds a give step as Give does, with the judgment the step hands the decision, or nil,
// and the receipt's decider by, each derived by hand (composition-7, receipt-11).
func (s *Scene) GiveJudged(at string, from Key, seq int64, message Obj, to Key, judgment Obj, verdict, reason string, by any) string {
	m := Canon(message)
	env := Envelope(seq, m, from)
	p := PartOf(message)
	meta := p["metadata"].(Obj)[Extension].(Obj)
	r, id := s.Write("crossing", from, to, meta["kind"].(string), Digest(p), at, from, seq, Hash(m), verdict, reason, by)
	step := Obj{
		"op": "give", "at": at, "as": from.Name,
		"messageHex": HexOf(m), "envelopeHex": HexOf(Canon(env)),
		"expect": Obj{"verdict": verdict, "reason": reason, "receiptId": id, "receipt": r},
	}
	if judgment != nil {
		step["judge"] = judgment
	}
	s.steps = append(s.steps, step)
	d := Obj{"messageId": message["messageId"], "kind": meta["kind"], "salt": meta["salt"]}
	_, body := Body(p)
	d[body] = p[body]
	if re, ok := Ext(message)["re"]; ok {
		d["re"] = re
	}
	if mt, ok := p["mediaType"]; ok {
		d["mediaType"] = mt
	}
	switch verdict {
	case "pass":
		s.queue(to, d, r, id)
	case "escalate":
		s.pending = append(s.pending, id)
		s.held[id] = heldPart{asker: from, receiver: to, eseq: seq, ehash: Hash(m),
			kind: meta["kind"].(string), digest: Digest(p), delivery: d, receipt: r}
	}
	return id
}

// queue queues for to a delivery of the part's members, with the receipt that passed it
// (crossing-3).
func (s *Scene) queue(to Key, part Obj, receipt Obj, id string) {
	d := Copy(part).(Obj)
	d["receiptId"], d["receipt"] = id, receipt
	s.queues[to.Name] = append(s.queues[to.Name], d)
}

// closeQuestion takes a question off the open list.
func (s *Scene) closeQuestion(q string) {
	var open []any
	for _, p := range s.pending {
		if p != q {
			open = append(open, p)
		}
	}
	s.pending = open
	delete(s.held, q)
}

// A Verdict is a receipt's verdict and reason, as a generator derives them by hand.
type Verdict struct{ Verdict, Reason string }

func outcome(v Verdict, r Obj, id string) Obj {
	return Obj{"verdict": v.Verdict, "reason": v.Reason, "receiptId": id, "receipt": r}
}

// An Answer is a resolve step of an accept or a change (conformance-vectors-6): its time, its
// question, its action, the answer's text or the version, and the salt the answerer minted for it,
// in hex; the answerer, who signs the call and is named in the step's by unless OmitBy, when the
// runner answers as the call's from; the resolution's verdict and reason, derived by hand; whether
// the call asks for release; and the release receipt's verdict and reason, or nil when none is
// written.
type Answer struct {
	At, Question, Action, Text, Salt string
	By                               Key
	OmitBy                           bool
	Verdict                          Verdict
	Release                          bool
	Released                         *Verdict
	// Call, when set, is the call the step gives in place of the one By would sign.
	Call Obj
}

// Resolve adds an accept or a change and writes what maybe-3, maybe-8 to maybe-12 say it writes: the
// answerer's signed call, the step's callHex; the answer or the version, a part of kind answer or
// changed-version formed with messageId answer: or change: and the question's id, its re the
// question, its text body and the step's salt, and the call inside its delivery; its resolution
// receipt from the answerer to the asker, echoing the question's envelope and naming the call as its
// decider; on a pass, the part queued for the asker; and when the generator names a release receipt,
// that receipt, of the same decider: after an accept, a crossing receipt from the asker to the
// receiver naming the asked part, which on a pass queues the held part as given with the call inside
// its delivery; after a change, a resolution receipt from the answerer to the receiver naming the
// version, which on a pass queues the version. The question closes. It returns the call.
func (s *Scene) Resolve(a Answer) Obj {
	h := s.held[a.Question]
	call := a.Call
	if call == nil {
		call = s.Call(a.Question, a.Action, a.Text, a.Salt, a.Release, a.By)
	}
	by := ByAnswer(call)
	kind, prefix := "answer", "answer:"
	if a.Action == "changed" {
		kind, prefix = "changed-version", "change:"
	}
	part := Obj{"messageId": prefix + a.Question, "kind": kind, "re": a.Question, "salt": a.Salt, "text": a.Text, "answer": call}
	digest := Sum(FromHex(a.Salt), []byte(a.Text))
	r, id := s.Write("resolution", a.By, h.asker, kind, digest, a.At, h.asker, h.eseq, h.ehash, a.Verdict.Verdict, a.Verdict.Reason, by)
	expect := outcome(a.Verdict, r, id)
	if a.Verdict.Verdict == "pass" {
		s.queue(h.asker, part, r, id)
	}
	if a.Released != nil {
		var rr Obj
		var rid string
		if a.Action == "accepted" {
			rr, rid = s.Write("crossing", h.asker, h.receiver, h.kind, h.digest, a.At, h.asker, h.eseq, h.ehash, a.Released.Verdict, a.Released.Reason, by)
			if a.Released.Verdict == "pass" {
				held := Copy(h.delivery).(Obj)
				held["answer"] = call
				s.queue(h.receiver, held, rr, rid)
			}
		} else {
			rr, rid = s.Write("resolution", a.By, h.receiver, kind, digest, a.At, h.asker, h.eseq, h.ehash, a.Released.Verdict, a.Released.Reason, by)
			if a.Released.Verdict == "pass" {
				s.queue(h.receiver, part, rr, rid)
			}
		}
		expect["release"] = outcome(*a.Released, rr, rid)
	}
	step := Obj{"op": "resolve", "at": a.At, "callHex": HexOf(Canon(call)), "answer": a.Text, "salt": a.Salt, "expect": expect}
	if !a.OmitBy {
		step["by"] = a.By.Name
	}
	s.steps = append(s.steps, step)
	s.closeQuestion(a.Question)
	return call
}

// Close adds a decline or a cancel signed by signer and named in the step's by, or an undeliverable,
// for which signer is nil (maybe-4), as CloseAs does.
func (s *Scene) Close(at, question, action string, signer *Key, from Key) {
	if signer == nil {
		s.CloseAs(at, question, action, Key{}, false, from)
		return
	}
	s.CloseAs(at, question, action, *signer, true, from)
}

// CloseAs adds a decline, a cancel or an undeliverable (maybe-4): a resolution receipt from from, the
// question's first answerer or the mediator, derived by hand, to the asker, block with the action as
// its reason, naming the asked part and echoing the question's envelope. A decline or a cancel is a
// call signed by signer, which is its decider, and named in the step's by when named; an
// undeliverable is the mediator's finding, a step of the question and undeliverable alone, whose
// decider is the mediator (receipt-11). Nothing is queued, and the question closes.
func (s *Scene) CloseAs(at, question, action string, signer Key, named bool, from Key) {
	h := s.held[question]
	var step Obj
	var by any
	if action == "undeliverable" {
		step = Obj{"op": "resolve", "at": at, "question": question, "undeliverable": true}
		by = ByUndeliverable
	} else {
		call := s.Call(question, action, "", "", false, signer)
		step = Obj{"op": "resolve", "at": at, "callHex": HexOf(Canon(call))}
		if named {
			step["by"] = signer.Name
		}
		by = ByAnswer(call)
	}
	r, id := s.Write("resolution", from, h.asker, h.kind, h.digest, at, h.asker, h.eseq, h.ehash, "block", action, by)
	step["expect"] = outcome(Verdict{"block", action}, r, id)
	s.steps = append(s.steps, step)
	s.closeQuestion(question)
}

// Refuse adds a resolve step rejected with code, which writes nothing (section 6): for an accept or
// a change, the call by's key signs over text under salt, without release, given with them; for a
// decline or a cancel, by's call alone; and for an undeliverable, for which by is nil, the
// mediator's finding.
func (s *Scene) Refuse(at, question, action string, by *Key, text, salt, code string) {
	if action == "undeliverable" {
		s.steps = append(s.steps, Obj{"op": "resolve", "at": at, "question": question, "undeliverable": true, "expect": Obj{"rejection": code}})
		return
	}
	s.RefuseCall(at, s.Call(question, action, text, salt, false, *by), by, text, salt, code)
}

// RefuseCall adds a resolve step of the call, as the member by names, or as the call's from when by
// is nil, given with text and salt for an accept or a change, rejected with code.
func (s *Scene) RefuseCall(at string, call Obj, by *Key, text, salt, code string) {
	step := Obj{"op": "resolve", "at": at, "callHex": HexOf(Canon(call)), "expect": Obj{"rejection": code}}
	if by != nil {
		step["by"] = by.Name
	}
	if a := call["action"]; a == "accepted" || a == "changed" || text != "" || salt != "" {
		step["answer"], step["salt"] = text, salt
	}
	s.steps = append(s.steps, step)
}

// A Timeout is a question a tick closes: its id, its deadline and the receipt's from, the first
// answerer or the mediator, each derived by hand.
type Timeout struct {
	Question, Deadline string
	From               Key
}

// Tick adds a tick step at at and writes, in order, a timeout for each question the generator says
// it reaches (maybe-5): a resolution receipt from the first answerer to the asker, block, timeout,
// at the deadline, naming the asked part, echoing the question's envelope and naming the deadline as
// its decider.
func (s *Scene) Tick(at string, closes ...Timeout) {
	list := []any{}
	for _, c := range closes {
		h := s.held[c.Question]
		r, id := s.Write("resolution", c.From, h.asker, h.kind, h.digest, c.Deadline, h.asker, h.eseq, h.ehash, "block", "timeout", ByDeadline)
		list = append(list, outcome(Verdict{"block", "timeout"}, r, id))
		s.closeQuestion(c.Question)
	}
	s.steps = append(s.steps, Obj{"op": "tick", "at": at, "expect": Obj{"receipts": list}})
}

// Reject adds a give step the door rejects with code: it writes nothing and changes nothing
// (crossing-7). The message and envelope are given as the members named, each in one of its forms.
func (s *Scene) Reject(at string, as Key, code string, message, envelope Obj) {
	st := Obj{"op": "give", "at": at, "as": as.Name, "expect": Obj{"rejection": code}}
	for k, v := range message {
		st[k] = v
	}
	for k, v := range envelope {
		st[k] = v
	}
	s.steps = append(s.steps, st)
}

// Get adds a fetch by the key's party: the first delivery queued for it, which it takes, or none.
func (s *Scene) Get(as Key) {
	q := s.queues[as.Name]
	expect := Obj{"none": true}
	if len(q) > 0 {
		expect = Obj{"delivery": q[0]}
		s.queues[as.Name] = q[1:]
		if len(q) == 1 {
			delete(s.queues, as.Name)
		}
	}
	s.steps = append(s.steps, Obj{"op": "get", "as": as.Name, "expect": expect})
}

// GetRejected adds a fetch the door rejects with code.
func (s *Scene) GetRejected(as Key, code string) {
	s.steps = append(s.steps, Obj{"op": "get", "as": as.Name, "expect": Obj{"rejection": code}})
}

// Vector writes the scenario vector: the keys by name, the founding set, its time, the steps and the
// final state.
func (s *Scene) Vector(name, tests string, set Obj) Vector {
	keys := Obj{}
	for n, k := range s.Keys {
		keys[n] = k.Seed
	}
	delivered := Obj{}
	for n, q := range s.queues {
		if len(q) > 0 {
			delivered[n] = append([]any(nil), q...)
		}
	}
	receipts := append([]any{}, s.written...)
	pending := append([]any{}, s.pending...)
	return Vector{Name: name, Tests: tests, Operation: "scenario", Members: Obj{
		"keys": keys, "mediator": s.Mediator.Name, "founding": set, "foundedAt": s.FoundedAt,
		"steps": append([]any(nil), s.steps...),
		"final": Obj{"receipts": receipts, "delivered": delivered, "pending": pending},
	}}
}

// Written returns every receipt the scene wrote, in order.
func (s *Scene) Written() []Obj {
	out := make([]Obj, len(s.written))
	for i, w := range s.written {
		out[i] = w.(Obj)["receipt"].(Obj)
	}
	return out
}

// Statements decodes a receipt's statement from its payload.
func StatementOf(receipt Obj) []byte { return FromB64(receipt["payload"].(string)) }

// Delivered returns the deliveries still queued for the key's party, in order.
func (s *Scene) Delivered(k Key) []Obj {
	var out []Obj
	for _, d := range s.queues[k.Name] {
		out = append(out, d.(Obj))
	}
	return out
}
