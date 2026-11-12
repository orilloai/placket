package kit

import "sort"

// Times every generator uses: the founding time and the bindings' end.
const (
	FoundedAt = "2026-11-12T08:00:00Z"
	Until     = "2027-11-12T00:00:00Z"
)

// Card is the card of every founding set: spec 0.1, a kernel named by its version and the hash of
// its name, no judges and no checks (setup-contract-4).
func Card() Obj {
	return Obj{
		"spec":   "0.1",
		"kernel": Obj{"version": "0.1.0", "hash": HashOf("placket reference decision module 0.1")},
		"judges": []any{},
		"checks": []any{},
	}
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

// Binding builds a principal's binding of an actor to roles under a mediator until a time.
func Binding(actor, principal, mediator Key, until string, roles ...string) Obj {
	return Signed(Obj{
		"actor": actor.Ref, "principal": principal.Ref, "roles": strs(roles),
		"mediator": mediator.Ref, "until": until,
	}, TypeBinding, principal)
}

// A Party is a member of a set: its key, the roles its binding gives it and its terms.
type Party struct {
	Key   Key
	Roles []string
	Terms []Obj
}

// A Set is a founding set as a generator describes it.
type Set struct {
	Mediator, Operator, Principal Key
	Roles                         []string
	Parties                       []Party
}

// Build writes the set: the policy, the mediator, the card, and each party's declaration and binding,
// both lists in actor-reference order (setup-contract-9, setup-contract-10).
func (s Set) Build() Obj {
	var decls, binds []Obj
	for _, p := range s.Parties {
		decls = append(decls, Declaration(p.Key, p.Terms...))
		binds = append(binds, Binding(p.Key, s.Principal, s.Mediator, Until, p.Roles...))
	}
	return FoundingSet(Obj{"roles": strs(s.Roles), "principals": []any{s.Principal.Ref}},
		Obj{"actor": s.Mediator.Ref, "operator": s.Operator.Ref, "runs": "process"}, Card(), decls, binds)
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

// Message builds a crossing message of one part, from a member, to a receiver under an Accord
// (crossing-1).
func Message(id, accord string, to Key, p Obj) Obj {
	return Obj{
		"messageId":  id,
		"role":       "ROLE_AGENT",
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
	// Mediator is the predicate's mediator; empty means the signing key's.
	Mediator string
}

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
	chains       map[[2]string]link
	written      []any
	queues       map[string][]any
	pending      []any
	steps        []any
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
	}
	for _, k := range keys {
		s.Keys[k.Name] = k
	}
	s.Keys[mediator.Name] = mediator
	return s
}

// Write writes the next receipt of the pair from, to: its seq one above the pair's last and its prev
// that receipt's id (receipt-5).
func (s *Scene) Write(kind string, from, to Key, name, digest, at string, sender Key, eseq int64, ehash, verdict, reason string) (Obj, string) {
	pair := [2]string{from.Ref, to.Ref}
	last := s.chains[pair]
	st := Statement(name, digest, Pred{
		Accord: s.Accord, Card: s.Card, Kind: kind, From: from.Ref, To: to.Ref,
		Seq: last.seq + 1, Prev: last.id, At: at,
		Sender: sender.Ref, ESeq: eseq, EHash: ehash, Verdict: verdict, Reason: reason,
	}, s.Mediator)
	r, id := Receipt(Canon(st), s.Mediator)
	s.chains[pair] = link{seq: last.seq + 1, id: id}
	s.written = append(s.written, Obj{"receiptId": id, "receipt": r})
	return r, id
}

// Give adds a give step from the sender at seq whose verdict and reason the generator derived by
// hand, writes its receipt, and queues the part for its receiver on a pass or holds it as a question
// on an escalate (maybe-1, crossing-3). It returns the receipt's id.
func (s *Scene) Give(at string, from Key, seq int64, message Obj, to Key, verdict, reason string) string {
	m := Canon(message)
	env := Envelope(seq, m, from)
	p := PartOf(message)
	meta := p["metadata"].(Obj)[Extension].(Obj)
	r, id := s.Write("crossing", from, to, meta["kind"].(string), Digest(p), at, from, seq, Hash(m), verdict, reason)
	s.steps = append(s.steps, Obj{
		"op": "give", "at": at, "as": from.Name,
		"messageHex": HexOf(m), "envelopeHex": HexOf(Canon(env)),
		"expect": Obj{"verdict": verdict, "reason": reason, "receiptId": id, "receipt": r},
	})
	switch verdict {
	case "pass":
		d := Obj{"receiptId": id, "receipt": r, "messageId": message["messageId"], "kind": meta["kind"], "salt": meta["salt"]}
		_, body := Body(p)
		d[body] = p[body]
		if re, ok := Ext(message)["re"]; ok {
			d["re"] = re
		}
		s.queues[to.Name] = append(s.queues[to.Name], d)
	case "escalate":
		s.pending = append(s.pending, id)
	}
	return id
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
		"keys": keys, "mediator": s.Mediator.Name, "founding": set, "foundedAt": FoundedAt,
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
