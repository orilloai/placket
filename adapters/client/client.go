// Package client is one party's client of the mediator API: the calls of a window's door, made as
// one key, for one Accord. It is what the command line's socket commands and placket mcp call. It
// signs each call with the key read from the party's key file; owns the envelope sequence numbers
// (crossing-4) and the state directory's lock, in the command line's state directory, so the command
// line and a client take turns on one key (state-lock); persists a fetched delivery to the state
// directory before returning it; and checks every reply: a receipt names its own Accord, a delivery
// is what its receipt says, a question shown is the part its ask names, and a view shown is the
// key's own view of its Accord, which it keeps, as it keeps the acceptance it signs of it.
//
// The calls and their replies are described, for a client in any language, by the schemas under
// api/ (docs/reference/mediator-api.md); this package takes the request layer, what a caller asks
// for, and makes the wire calls from it. A refusal by the window, its rejection, error or door code,
// comes back in the reply and is not an error of this package. The labels config, which names the
// mediators a user reaches, is config.go.
package client

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// Code says why a client's call failed on the client's side.
type Code string

// The codes of Error.
const (
	// CodeBadConfig: the labels config is not in its format (api/config/mediators.json).
	CodeBadConfig Code = "bad-config"
	// CodeLabelTaken: two entries of the labels config have one label.
	CodeLabelTaken Code = "label-taken"
	// CodeMediatorCollision: two entries of the labels config would use one socket of two Accords,
	// one state directory, or be named by one Accord id.
	CodeMediatorCollision Code = "mediator-collision"
	// CodeUnknownMediator: no entry of the labels config, or no mediator served, has the name.
	CodeUnknownMediator Code = "unknown-mediator"
	// CodeReply: the window's reply is not what the call asked for.
	CodeReply Code = "reply"
	// CodeAsked: what the window shows of the question is not the part its ask's receipt names, so
	// no answer to it is signed.
	CodeAsked Code = "asked"
	// CodeRand: the source of randomness failed.
	CodeRand Code = "rand"
	// CodeEnvelope: an envelope could not be signed.
	CodeEnvelope Code = "envelope"
	// CodeView: the key keeps no view of the Accord to accept, so accept signs nothing; view keeps one
	// (setup-contract-15).
	CodeView Code = "view"
)

// Error is a call that failed on the client's side: its code, and for a check across the labels
// config's entries, the two entries at fault, by their index in its list. It holds nothing of a
// call, a reply or a key (error-text-no-content).
type Error struct {
	Code    Code
	Entries []int
}

// Error returns "client: ", the code and the entries' indexes.
func (e *Error) Error() string {
	s := "client: " + string(e.Code)
	for _, i := range e.Entries {
		s += " " + strconv.Itoa(i)
	}
	return s
}

func fail(c Code) error { return &Error{Code: c} }

// A Mediator is what a client needs to reach one mediator: the Accord id, the key file, the window's
// socket and the state directory; and, from the labels config, its label and its founding set's
// path, empty when it has none or was named by flags.
type Mediator struct {
	Label, Accord, Key, Founding, Socket, State string
}

// Name is how a mediator is named to a person and in a call of the MCP tools: its label, or the
// Accord id when it has none.
func (m Mediator) Name() string {
	if m.Label != "" {
		return m.Label
	}
	return m.Accord
}

// Options are what a client takes from the world.
type Options struct {
	// Now is the clock; Rand the source of message ids and salts; Nonces the source of each call's
	// nonce, nil for Rand.
	Now    func() time.Time
	Rand   io.Reader
	Nonces io.Reader
	// Stderr takes the state directory's repairs.
	Stderr io.Writer
	// Timeout bounds one call, as local.Client's does; zero is its default.
	Timeout time.Duration
}

// A Client calls one mediator as one key.
type Client struct {
	m     Mediator
	key   signing.Key
	dir   *state.Dir
	door  *local.Client
	rand  io.Reader
	locks int
}

// persisted, when set, is called by Get once a delivery is persisted and before it is returned. Only
// a test sets it.
var persisted func()

// Open reads the key and opens the state directory, by default the command line's,
// state.DefaultPath(m.Key, m.Accord). It checks nothing it would need a reply for.
func Open(m Mediator, o Options) (*Client, error) {
	if err := local.CheckSocket(m.Socket); err != nil {
		return nil, err
	}
	k, err := keyfile.Read(m.Key)
	if err != nil {
		return nil, err
	}
	if m.State == "" {
		m.State = state.DefaultPath(m.Key, m.Accord)
	}
	stderr := o.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	d, err := state.Open(m.State, k.Reference(), stderr)
	if err != nil {
		return nil, err
	}
	nonces := o.Nonces
	if nonces == nil {
		nonces = o.Rand
	}
	return &Client{m: m, key: k, dir: d, rand: o.Rand,
		door: &local.Client{Socket: m.Socket, Key: k, Accord: m.Accord, Now: o.Now, Rand: nonces, Timeout: o.Timeout}}, nil
}

// Mediator is the mediator the client reaches.
func (c *Client) Mediator() Mediator { return c.m }

// Accord is the Accord id the client calls under.
func (c *Client) Accord() string { return c.m.Accord }

// Reference is the client's key's actor reference.
func (c *Client) Reference() string { return c.key.Reference() }

// Dir is the client's state directory.
func (c *Client) Dir() *state.Dir { return c.dir }

// Lock takes the state directory's lock, or counts one more hold of it when the client holds it
// already. A command that writes to the state directory around a client's calls holds it across them.
func (c *Client) Lock() error {
	if c.locks == 0 {
		if err := c.dir.Lock(); err != nil {
			return err
		}
	}
	c.locks++
	return nil
}

// Unlock lets go of one hold of the lock, and of the lock with the last.
func (c *Client) Unlock() {
	if c.locks == 0 {
		return
	}
	if c.locks--; c.locks == 0 {
		c.dir.Unlock()
	}
}

// Refused reports a reply that is a refusal: the member that names it, rejection, error or door, and
// its code.
func Refused(r local.Reply) (member, code string, ok bool) {
	switch {
	case r.Rejection != "":
		return "rejection", string(r.Rejection), true
	case r.Error != "":
		return "error", string(r.Error), true
	case r.Door != "":
		return "door", string(r.Door), true
	case r.Founding != "":
		return "founding", r.Founding, true
	}
	return "", "", false
}

// View asks the window for the key's own view of the Accord (setup-contract-14) and keeps it as
// view.json in the state directory, under its lock, replacing any view kept before. A view that is
// not this key's view of this Accord is CodeReply, and nothing is kept. A refusal is in the reply.
func (c *Client) View(ctx context.Context) (local.Reply, error) {
	if err := c.Lock(); err != nil {
		return local.Reply{}, err
	}
	defer c.Unlock()
	rep, err := c.door.CallContext(ctx, local.OpView, nil)
	if err != nil {
		return local.Reply{}, err
	}
	if _, _, refused := Refused(rep); refused {
		return rep, nil
	}
	var v struct {
		Accord string `json:"accord"`
		Viewer string `json:"viewer"`
	}
	if rep.View == nil || json.Unmarshal(rep.View, &v) != nil || v.Accord != c.m.Accord || v.Viewer != c.key.Reference() {
		return local.Reply{}, fail(CodeReply)
	}
	if err := c.dir.KeepView(rep.View); err != nil {
		return local.Reply{}, err
	}
	return rep, nil
}

// Accept signs the key's acceptance of the view the state directory keeps (founding.Accept,
// setup-contract-15) and hands it to the window, under the state directory's lock. When the window
// takes it, the acceptance is kept as acceptance.json beside the view, and it returns the acceptance
// with the reply, which says whether the Accord is founded. With no view kept it is CodeView. A
// refusal is in the reply, and nothing is kept.
func (c *Client) Accept(ctx context.Context) (local.Reply, []byte, error) {
	if err := c.Lock(); err != nil {
		return local.Reply{}, nil, err
	}
	defer c.Unlock()
	view, ok, err := c.dir.View()
	if err != nil {
		return local.Reply{}, nil, err
	}
	if !ok {
		return local.Reply{}, nil, fail(CodeView)
	}
	acc, err := founding.Accept(c.m.Accord, view, c.key)
	if err != nil {
		return local.Reply{}, nil, fail(CodeView)
	}
	rep, err := c.door.CallContext(ctx, local.OpAccept, local.AcceptArgs(acc))
	if err != nil {
		return local.Reply{}, nil, err
	}
	if _, _, refused := Refused(rep); refused {
		return rep, nil, nil
	}
	if !rep.IsAccept {
		return local.Reply{}, nil, fail(CodeReply)
	}
	if err := c.dir.KeepAcceptance(acc); err != nil {
		return local.Reply{}, nil, err
	}
	return rep, acc, nil
}

// A Body is the body of a part a client gives, in one of three forms: a text, a raw body's bytes, or
// a data body's JSON, in canonical form.
type Body struct {
	Form string
	Text string
	Raw  []byte
	Data []byte
}

// The forms of a body.
const (
	FormText = "text"
	FormRaw  = "raw"
	FormData = "data"
)

// A Message is the one part a give sends: its receiver, kind, body, and the re and topic it may have.
type Message struct {
	To, Kind  string
	Body      Body
	Re, Topic string
}

// uuid is a fresh lowercase UUID version 4.
func uuid(rand io.Reader) (string, bool) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand, b); err != nil {
		return "", false
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], true
}

// canonicalJSON is the canonical bytes of v.
func canonicalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	c, _ := canonical.Form(b)
	return c
}

// message builds a crossing message of one part, as a member gives it: in ROLE_USER, its id a fresh
// UUID, under a salt of SaltBytes fresh random bytes (crossing-1, crossing-2, crossing-3).
func (c *Client) message(m Message) ([]byte, bool) {
	id, ok := uuid(c.rand)
	if !ok {
		return nil, false
	}
	salt := make([]byte, mediator.SaltBytes)
	if _, err := io.ReadFull(c.rand, salt); err != nil {
		return nil, false
	}
	ext := map[string]any{"accord": c.m.Accord, "to": m.To}
	if m.Re != "" {
		ext["re"] = m.Re
	}
	if m.Topic != "" {
		ext["topic"] = m.Topic
	}
	var value any
	switch m.Body.Form {
	case FormText:
		value = m.Body.Text
	case FormRaw:
		value = base64.StdEncoding.EncodeToString(m.Body.Raw)
	default:
		value = json.RawMessage(m.Body.Data)
	}
	part := map[string]any{m.Body.Form: value, "metadata": map[string]any{mediator.Extension: map[string]any{"kind": m.Kind, "salt": hex.EncodeToString(salt)}}}
	b := canonicalJSON(map[string]any{
		"messageId": id, "role": mediator.RoleUser, "extensions": []string{mediator.Extension},
		"metadata": map[string]any{mediator.Extension: ext}, "parts": []any{part},
	})
	return b, b != nil
}

// Give gives one part as the client's key, under the state directory's lock (state-lock): at the
// next seq, above the seq file and every envelope recorded, so a give whose reply was lost never has
// its seq used again; its envelope recorded before the call is sent and removed again only on a
// refusal. With keep, it keeps the part it gave and its receipt. It returns the reply: its Outcome,
// or a refusal. A reply with neither is CodeReply.
func (c *Client) Give(ctx context.Context, m Message, keep bool) (local.Reply, error) {
	if err := c.Lock(); err != nil {
		return local.Reply{}, err
	}
	defer c.Unlock()
	seq, err := c.dir.Next()
	if err != nil {
		return local.Reply{}, err
	}
	msg, ok := c.message(m)
	if !ok {
		return local.Reply{}, fail(CodeRand)
	}
	hash, _ := canonical.TypedHash(msg)
	env := canonicalJSON(map[string]any{"seq": seq, "hash": hash})
	sig, err := signing.Sign(env, signing.TypeEnvelope, c.key)
	if err != nil {
		return local.Reply{}, fail(CodeEnvelope)
	}
	env = canonicalJSON(map[string]any{"seq": seq, "hash": hash, "sig": sig})
	if err := c.dir.AddEnvelope(seq, hash); err != nil {
		return local.Reply{}, err
	}
	rep, err := c.door.CallContext(ctx, local.OpGive, local.GiveArgs(msg, env))
	if err != nil {
		// No reply: the envelope stays, so a submission that crossed unseen shows as missing.
		return local.Reply{}, err
	}
	if _, _, refused := Refused(rep); refused {
		if err := c.dir.DropEnvelope(seq, hash); err != nil {
			return local.Reply{}, err
		}
		return rep, nil
	}
	if rep.Outcome == nil {
		return local.Reply{}, fail(CodeReply)
	}
	if err := c.dir.SetSeq(seq); err != nil {
		return local.Reply{}, err
	}
	if keep {
		k := canonicalJSON(map[string]any{"message": base64.StdEncoding.EncodeToString(msg), "receipt": json.RawMessage(rep.Outcome.Receipt)})
		if err := c.dir.Keep(k); err != nil {
			return local.Reply{}, err
		}
	}
	return rep, nil
}

// A Delivery is a delivery as a client reads it.
type Delivery struct {
	ReceiptID string          `json:"receiptId"`
	Receipt   json.RawMessage `json:"receipt"`
	MessageID string          `json:"messageId"`
	Kind      string          `json:"kind"`
	Salt      string          `json:"salt"`
	Re        string          `json:"re"`
	Text      *string         `json:"text"`
	Raw       *string         `json:"raw"`
	Data      json.RawMessage `json:"data"`
	MediaType *string         `json:"mediaType"`
	// Answer is the answer call inside a delivery an answer queued (maybe-12).
	Answer json.RawMessage `json:"answer"`
}

// Format prints the Delivery as a fixed text, whatever the verb: it holds a body and a salt
// (secrets-never-print).
func (Delivery) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("client.Delivery")) }

// Bytes is the delivery's body: a text's UTF-8, a raw body's decoded bytes, a data body's canonical
// bytes.
func (d Delivery) Bytes() []byte {
	switch {
	case d.Text != nil:
		return []byte(*d.Text)
	case d.Raw != nil:
		b, _ := base64.StdEncoding.DecodeString(*d.Raw)
		return b
	}
	c, _ := canonical.Form(d.Data)
	return c
}

// OneBody reports whether the delivery holds exactly one body, a raw one in strict standard base64.
func (d Delivery) OneBody() bool {
	bodies := 0
	for _, has := range []bool{d.Text != nil, d.Raw != nil, len(d.Data) > 0} {
		if has {
			bodies++
		}
	}
	if d.Raw != nil {
		if _, err := base64.StdEncoding.Strict().DecodeString(*d.Raw); err != nil {
			return false
		}
	}
	return bodies == 1
}

// SaltBytes decodes a salt written in hex, and reports whether it is exactly SaltBytes long
// (crossing-2). A salt of any other length matches no receipt.
func SaltBytes(s string) ([]byte, bool) {
	b, err := hex.DecodeString(s)
	return b, err == nil && len(b) == mediator.SaltBytes
}

// about reports whether a delivery is what its receipt says it is, before anything of it is
// recorded, printed or written: a receipt that decodes (receipts.Decode, which verifies nothing),
// that passed, to this key under this Accord, of the id and kind the delivery names, whose digest
// is that of the delivery's salt, of exactly SaltBytes, and its one body. So a delivery's kind is a
// label and its receipt id a typed hash, whatever window sent it; reconcile and verify, under the
// mediator's key, are what show the receipt is the mediator's.
func (c *Client) about(d Delivery) bool {
	st, err := receipts.Decode(d.Receipt)
	if err != nil {
		return false
	}
	salt, ok := SaltBytes(d.Salt)
	p := st.Predicate
	// The answer call inside the delivery is the one its receipt names, when an answer queued it.
	answered := len(d.Answer) == 0 && p.By.Decider != receipts.ByAnswer ||
		len(d.Answer) > 0 && p.By.Decider == receipts.ByAnswer && answer.ID(d.Answer) == p.By.Answer
	return ok && answered && d.OneBody() && st.ID == d.ReceiptID && st.Subject.Name == d.Kind &&
		p.Verdict == string(composition.Pass) && p.To == c.key.Reference() && p.Accord == c.m.Accord &&
		receipts.Digest(salt, d.Bytes()) == st.Subject.Digest
}

// A Got is what a get returned: the reply, a refusal in it; the delivery fetched and its line of
// kept.jsonl, or nil when nothing was queued; and the close receipt the reply carried in place of a
// delivery, once the Accord is closed and the queue is empty (close-3).
type Got struct {
	Reply    local.Reply
	Delivery *Delivery
	Kept     []byte
	Close    []byte
}

// Get fetches the next delivery and records it, under the state lock, before it returns it: its
// receipt in delivered.jsonl, its salt in salts.jsonl, and, with keep, the delivery, the answer call
// inside it, in kept.jsonl. A delivery that is not what its receipt says is CodeReply, and nothing
// of it is recorded. Once the window has handed a delivery over it is no longer queued, so a record
// that cannot be written returns the delivery with the state directory's error, and the caller
// still has the body; the records after the one that failed are not written.
//
// With wait, in whole seconds, it first asks the window, without the lock, to hold the call until a
// delivery is queued for the key or the wait ends; such a call takes nothing, so a client that hangs
// up while it waits loses nothing. Once one is queued, or the window hands the close, it takes the
// lock and fetches as a get without wait does.
func (c *Client) Get(ctx context.Context, wait int, keep bool) (Got, error) {
	if wait > 0 {
		rep, err := c.door.CallContext(ctx, local.OpGet, map[string]any{"wait": wait})
		if err != nil {
			return Got{}, err
		}
		if _, _, refused := Refused(rep); refused || !rep.Queued && rep.Close == nil {
			if rep.Delivery != nil {
				return Got{}, fail(CodeReply)
			}
			return Got{Reply: rep}, nil
		}
	}
	if err := c.Lock(); err != nil {
		return Got{}, err
	}
	defer c.Unlock()
	return c.fetch(ctx, keep)
}

// fetch is Get without wait, under the lock the caller holds.
func (c *Client) fetch(ctx context.Context, keep bool) (Got, error) {
	rep, err := c.door.CallContext(ctx, local.OpGet, nil)
	if err != nil {
		return Got{}, err
	}
	if _, _, refused := Refused(rep); refused {
		return Got{Reply: rep}, nil
	}
	if rep.Delivery == nil {
		return Got{Reply: rep, Close: rep.Close}, nil
	}
	var d Delivery
	if !strict.Decode(rep.Delivery, &d) || !c.about(d) {
		return Got{}, fail(CodeReply)
	}
	kl := keptLine(rep.Delivery)
	g := Got{Reply: rep, Delivery: &d, Kept: kl}
	if err := c.dir.AddDelivered(d.Receipt); err != nil {
		return g, err
	}
	if err := c.dir.AddSalt(d.ReceiptID, d.Salt); err != nil {
		return g, err
	}
	if keep {
		if err := c.dir.Keep(kl); err != nil {
			return g, err
		}
	}
	if persisted != nil {
		persisted()
	}
	return g, nil
}

// keptLine is a fetched delivery's line of kept.jsonl: the delivery as it came, the answer call
// inside it when an answer queued it.
func keptLine(delivery []byte) []byte {
	var o map[string]json.RawMessage
	if !strict.Decode(delivery, &o) {
		return nil
	}
	return canonicalJSON(o)
}

// Shown is what a questions call showed: the reply, a refusal in it; the questions the key may
// answer, each the part its ask's receipt names; the ids of those the window showed that are not
// (Misstated), left out of Questions, as the window wrote them; whether it left some out to keep the
// reply under local.MaxReply (More); the cursor to pass back as after, which is past the questions
// left out too; and the close, on a closed Accord.
type Shown struct {
	Reply     local.Reply
	Questions []local.Asked
	Misstated []string
	More      bool
	Cursor    string
	Close     []byte
}

// Questions asks for the open questions the key may answer, without the lock: it reads no seq and
// writes nothing. With after, a cursor of an earlier reply, it asks only for those opened since;
// with wait, in whole seconds, the window holds the call until there is one or the wait ends. A
// reply that is neither a refusal nor questions is CodeReply.
func (c *Client) Questions(ctx context.Context, wait int, after string) (Shown, error) {
	args := map[string]any{}
	if wait > 0 {
		args["wait"] = wait
	}
	if after != "" {
		args["after"] = after
	}
	rep, err := c.door.CallContext(ctx, local.OpQuestions, args)
	if err != nil {
		return Shown{}, err
	}
	if _, _, refused := Refused(rep); refused {
		return Shown{Reply: rep}, nil
	}
	if !rep.IsQuestions {
		return Shown{}, fail(CodeReply)
	}
	s := Shown{Reply: rep, Questions: []local.Asked{}, More: rep.More, Cursor: rep.Cursor, Close: rep.Close}
	for _, q := range rep.Questions {
		if !c.Asked(q) {
			s.Misstated = append(s.Misstated, q.ID)
			continue
		}
		s.Questions = append(s.Questions, q)
	}
	return s, nil
}

// Asked reports whether a question the window shows is the part its ask's receipt names: the
// receipt is the question's, under this Accord, of the kind shown, and its subject digest is the
// salted digest of the body shown under the salt shown (crossing-3, maybe-1).
func (c *Client) Asked(q local.Asked) bool {
	st, err := receipts.Decode(q.Receipt)
	return err == nil && st.ID == q.ID && st.Predicate.Accord == c.m.Accord && st.Subject.Name == q.Kind &&
		receipts.Digest(q.Salt, askedBody(q)) == st.Subject.Digest
}

// askedBody is the bytes of an asked part's body as the digest reads them: a text's UTF-8, a data
// body's canonical bytes, a raw body's bytes.
func askedBody(q local.Asked) []byte {
	if q.Raw != nil {
		return q.Raw
	}
	if q.Message != nil {
		return []byte(*q.Message)
	}
	return nil
}

// Result asks how a question the key asked closed, without the lock: the reply's Outcome and
// Release, nil while it is open or not the key's, or a refusal. With wait, in whole seconds, the
// window holds the call until the question closes or the wait ends.
func (c *Client) Result(ctx context.Context, question string, wait int) (local.Reply, error) {
	args := map[string]any{"question": question}
	if wait > 0 {
		args["wait"] = wait
	}
	return c.door.CallContext(ctx, local.OpResult, args)
}

// An Own is what FetchOwn found: the answer or the version to the question, nil when it was not
// fetched; whether it came from what the key had kept; every other delivery it fetched on the way, in
// the order fetched, each recorded and kept; and the reply of a fetch that was refused. With an
// error, Unkept is the kept.jsonl line of the delivery the window handed over whose record could not
// be written, the answer or the last of the others, which is not kept, for a caller that cannot hand
// its body over to keep; nil when there is none.
type Own struct {
	Delivery *Delivery
	Kept     bool
	Others   []*Delivery
	Reply    local.Reply
	Unkept   []byte
}

// keptDelivery is a line of kept.jsonl read as a fetched part. An answer line, {"answer","salt"},
// reads as one with no receipt id, which no reader takes for a part.
type keptDelivery struct {
	Delivery
}

// FetchOwn finds the answer or the version to the question, the delivery the closing receipt queued,
// under the state lock: first among the parts kept, then by fetching, every delivery fetched recorded
// and kept. On an error, Own holds what was fetched before it, and the delivery whose record could
// not be written, if the window had handed one over (Get).
func (c *Client) FetchOwn(ctx context.Context, question, closing string) (Own, error) {
	if err := c.Lock(); err != nil {
		return Own{}, err
	}
	defer c.Unlock()
	kept, err := c.dir.Kept()
	if err != nil {
		return Own{}, err
	}
	for _, l := range kept {
		var k keptDelivery
		if strict.Decode(l, &k) && IsOwn(&k.Delivery, question, closing) {
			return Own{Delivery: &k.Delivery, Kept: true}, nil
		}
	}
	var own Own
	for {
		g, err := c.fetch(ctx, true)
		if err != nil {
			// A delivery the window handed over is no longer queued: it is returned with the error,
			// as the answer or among the others, so the caller still has its body.
			if g.Delivery != nil {
				own.Unkept = g.Kept
				if IsOwn(g.Delivery, question, closing) {
					own.Delivery = g.Delivery
				} else {
					own.Others = append(own.Others, g.Delivery)
				}
			}
			return own, err
		}
		if _, _, refused := Refused(g.Reply); refused {
			own.Reply = g.Reply
			return own, nil
		}
		if g.Delivery == nil {
			return own, nil
		}
		if IsOwn(g.Delivery, question, closing) {
			own.Delivery = g.Delivery
			return own, nil
		}
		own.Others = append(own.Others, g.Delivery)
	}
}

// IsOwn reports whether the delivery is the answer or the version to the question: the part queued
// by the closing receipt, whose id it names. A part of that kind and re that a member gave has a
// receipt of its own, and is not.
func IsOwn(d *Delivery, question, closing string) bool {
	return d.ReceiptID == closing && d.Re == question && d.Text != nil &&
		(d.Kind == receipts.NameAnswer || d.Kind == receipts.NameChanged)
}

// An Answering is what one resolve signs and sends: the question, the action, for an accept or a
// change the text and whether it asks for release, and whether the key keeps the call it signed.
type Answering struct {
	Question string
	Action   answer.Action
	Text     string
	Release  bool
	Keep     bool
}

// A Signed is what a resolve signed and minted: the answer call's bytes, and the answer's salt, nil
// for a decline or a cancel.
type Signed struct{ Call, Salt []byte }

// Resolve answers the question (maybe-11), under the state lock: for an accept, a change or a decline
// it first checks the question the window shows, if it shows it (shown); for an accept or a change it mints the
// answer's salt, SaltBytes random bytes; it signs the answer call with the key, the answer's salted
// digest in it and never its text; with keep it keeps the call and the salt; and it sends the call,
// with the text and the salt beside it. It returns the reply, its Outcome and Release, or a refusal,
// and what it signed. A cancel is the asker's withdrawal of its own question (maybe-6), and checks
// nothing shown.
func (c *Client) Resolve(ctx context.Context, a Answering) (local.Reply, Signed, error) {
	if err := c.Lock(); err != nil {
		return local.Reply{}, Signed{}, err
	}
	defer c.Unlock()
	if a.Action != answer.Cancelled {
		if rep, err := c.shown(ctx, a.Question); err != nil || rep != nil {
			if rep != nil {
				return *rep, Signed{}, nil
			}
			return local.Reply{}, Signed{}, err
		}
	}
	call := answer.Call{Accord: c.m.Accord, Question: a.Question, Action: a.Action}
	var salt []byte
	if a.Action == answer.Accepted || a.Action == answer.Changed {
		salt = make([]byte, mediator.SaltBytes)
		if _, err := io.ReadFull(c.rand, salt); err != nil {
			return local.Reply{}, Signed{}, fail(CodeRand)
		}
		call.Digest, call.Release = receipts.Digest(salt, []byte(a.Text)), a.Release
	}
	signed, err := answer.Sign(call, c.key)
	if err != nil {
		return local.Reply{}, Signed{}, err
	}
	if a.Keep && salt != nil {
		if err := c.dir.Keep(canonicalJSON(map[string]any{"answer": json.RawMessage(signed), "salt": hex.EncodeToString(salt)})); err != nil {
			return local.Reply{}, Signed{}, err
		}
	}
	rep, err := c.door.CallContext(ctx, local.OpResolve, local.ResolveArgs(signed, a.Text, salt))
	if err != nil {
		return local.Reply{}, Signed{}, err
	}
	if _, _, refused := Refused(rep); refused {
		return rep, Signed{}, nil
	}
	if rep.Outcome == nil {
		return local.Reply{}, Signed{}, fail(CodeReply)
	}
	return rep, Signed{Call: signed, Salt: salt}, nil
}

// Cancel withdraws a question the key asked: a resolve whose action is cancelled (maybe-6).
func (c *Client) Cancel(ctx context.Context, question string) (local.Reply, error) {
	rep, _, err := c.Resolve(ctx, Answering{Question: question, Action: answer.Cancelled})
	return rep, err
}

// shown reads the question as the window shows it to this key's answerer and checks it before
// anything is signed (Asked). A question the window shows that is not the part its ask's receipt
// names is CodeAsked: nothing is signed for a part that does not check. A question the window does
// not show, on any page of its questions, is closed or not this key's to answer, so the call goes on
// to the window, whose refusal says which (maybe-6, maybe-7). It returns a reply to stop on, a
// refusal or, on a closed Accord, the rejection closed, which the window would give right after
// membership (close-3); or nil.
func (c *Client) shown(ctx context.Context, question string) (*local.Reply, error) {
	after := ""
	for {
		s, err := c.Questions(ctx, 0, after)
		if err != nil {
			return nil, err
		}
		if _, _, refused := Refused(s.Reply); refused {
			return &s.Reply, nil
		}
		if s.Close != nil {
			return &local.Reply{Rejection: mediator.AccordClosed}, nil
		}
		for _, q := range s.Questions {
			if q.ID == question {
				return nil, nil
			}
		}
		for _, id := range s.Misstated {
			if id == question {
				return nil, fail(CodeAsked)
			}
		}
		if !s.More || s.Cursor == "" || s.Cursor == after {
			return nil, nil
		}
		after = s.Cursor
	}
}
