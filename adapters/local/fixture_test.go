package local_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

func key(t testing.TB, name string) signing.Key {
	t.Helper()
	seed := sha256.Sum256([]byte("placket door tests: key " + name))
	k, err := signing.NewKey(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func signed(t testing.TB, o map[string]any, typeURI string, k signing.Key) map[string]any {
	t.Helper()
	b, _ := json.Marshal(o)
	sig, err := signing.Sign(b, typeURI, k)
	if err != nil {
		t.Fatal(err)
	}
	c := map[string]any{"sig": sig}
	for n, v := range o {
		c[n] = v
	}
	return c
}

func term(stance, dir string, with, kinds []string) map[string]any {
	sel := make([]string, len(with))
	for i, w := range with {
		sel[i] = "role:" + w
	}
	return map[string]any{"stance": stance, "dir": dir, "with": sel, "select": map[string]any{"kind": kinds}}
}

func maybe(with, kinds []string, to string) map[string]any {
	t := term("maybe", "give", with, kinds)
	t["resolve"] = map[string]any{"to": []string{"role:" + to}, "timeout": 3600}
	return t
}

// askAndLog is the ask-and-log Accord of the worked example, with two more crossings: a note the
// proposer gives the applier freely, and a draft it asks about under a rule whose role, auditor,
// nobody holds. The mediator's key is named mediator; mediatorName names another for a second Accord.
func askAndLog(t testing.TB, mediatorName string) []byte {
	t.Helper()
	med, op, pr := key(t, mediatorName), key(t, "operator"), key(t, "principal")
	parties := []struct {
		name  string
		terms []map[string]any
	}{
		{"proposer", []map[string]any{
			maybe([]string{"applier"}, []string{"proposal"}, "decider"),
			maybe([]string{"applier"}, []string{"draft"}, "auditor"),
			term("want", "give", []string{"applier"}, []string{"note"}),
			term("want", "get", []string{"decider"}, []string{"answer", "changed-version", "comment"}),
		}},
		{"applier", []map[string]any{
			term("want", "get", []string{"proposer"}, []string{"proposal", "draft", "note"}),
			term("want", "get", []string{"decider"}, []string{"changed-version"}),
		}},
		{"decider", []map[string]any{
			term("want", "give", []string{"proposer"}, []string{"answer", "changed-version", "comment"}),
			term("want", "give", []string{"applier"}, []string{"changed-version"}),
		}},
	}
	// One principal stands behind every party; each binding names the typed hash of the policy.
	roles := []string{"proposer", "applier", "decider"}
	policy := map[string]any{"roles": roles, "principals": []any{map[string]any{"principal": pr.Reference(), "roles": roles}}}
	pb, _ := json.Marshal(policy)
	policyHash, err := canonical.TypedHash(pb)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		actor      string
		decl, bind map[string]any
	}
	var es []entry
	for _, p := range parties {
		k := key(t, p.name)
		terms := make([]any, len(p.terms))
		for i, x := range p.terms {
			terms[i] = x
		}
		es = append(es, entry{k.Reference(),
			signed(t, map[string]any{"actor": k.Reference(), "terms": terms}, signing.TypeDeclaration, k),
			signed(t, map[string]any{"actor": k.Reference(), "principal": pr.Reference(), "roles": []string{p.name},
				"policy": policyHash, "mediator": med.Reference(), "until": "2099-01-01T00:00:00Z"}, signing.TypeBinding, pr)})
	}
	sort.Slice(es, func(i, j int) bool { return canonical.Less(es[i].actor, es[j].actor) })
	var decls, binds []any
	for _, e := range es {
		decls, binds = append(decls, e.decl), append(binds, e.bind)
	}
	b, _ := json.Marshal(map[string]any{
		"policy":   policy,
		"mediator": map[string]any{"actor": med.Reference(), "operator": op.Reference(), "runs": "process"},
		"card": map[string]any{"spec": "0.1", "kernel": map[string]any{"version": "0.1.0", "hash": "sha256:" + hex.EncodeToString(make([]byte, 32))},
			"judges": []any{}, "checks": []any{}},
		"declarations": decls, "bindings": binds,
	})
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A clock a test moves: every reading gives the time it was set to.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

// counter is a scripted source of bytes: each read gives the next bytes of a hash chain of its name.
type counter struct {
	mu   sync.Mutex
	name string
	n    uint64
}

func (c *counter) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < len(p); i += 32 {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], c.n)
		c.n++
		h := sha256.Sum256(append([]byte(c.name), b[:]...))
		copy(p[i:], h[:])
	}
	return len(p), nil
}

// A running window, in this process.
type running struct {
	cancel context.CancelFunc
	done   chan error
	stderr *bytes.Buffer
	events *[]local.Event
	mu     *sync.Mutex
}

// shortDir is a directory whose path is short enough for a unix socket on every system.
func shortDir(t testing.TB) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "plk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// start runs a window and waits until it listens, or fails the test with what Run returned.
func start(t testing.TB, w local.Window) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	r := &running{cancel: cancel, done: make(chan error, 1), stderr: &bytes.Buffer{}, events: &[]local.Event{}, mu: &sync.Mutex{}}
	w.Ready = ready
	w.Stderr = &lockedWriter{mu: r.mu, b: r.stderr}
	w.Event = func(e local.Event) { r.mu.Lock(); *r.events = append(*r.events, e); r.mu.Unlock() }
	go func() { r.done <- local.Run(ctx, w) }()
	select {
	case <-ready:
	case err := <-r.done:
		cancel()
		t.Fatalf("the window did not start: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	t.Cleanup(func() { r.stop(t) })
	return r
}

// stop drops the window: its context ends, its files close, nothing else is cleaned up.
func (r *running) stop(t testing.TB) {
	if r.cancel == nil {
		return
	}
	r.cancel()
	r.cancel = nil
	select {
	case err := <-r.done:
		if err != nil {
			t.Errorf("the window ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("the window did not stop")
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	b  *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

// A world is one Accord's files, its clock and its salt and nonce sources.
type world struct {
	set     []byte
	accord  string
	socket  string
	journal string
	clock   *clock
	salts   *counter
	nonces  *counter
	seqs    map[string]int64
	// disk is what the world's windows write their journal through; nil is the system's.
	disk journal.Disk
}

func newWorld(t testing.TB) *world {
	t.Helper()
	set := askAndLog(t, "mediator")
	id, _ := canonical.TypedHash(set)
	d := shortDir(t)
	return &world{set: set, accord: id, socket: filepath.Join(d, "s"), journal: filepath.Join(d, "j"),
		clock: &clock{t: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}, salts: &counter{name: "salts"},
		nonces: &counter{name: "nonces"}, seqs: map[string]int64{}}
}

func (w *world) window(t testing.TB) local.Window {
	return local.Window{Key: key(t, "mediator"), Founding: w.set, Socket: w.socket, Journal: w.journal,
		Now: w.clock.Now, Rand: w.salts, Idle: 20 * time.Millisecond, Disk: w.disk}
}

func (w *world) client(t testing.TB, name string) *local.Client {
	return &local.Client{Socket: w.socket, Key: key(t, name), Accord: w.accord, Now: w.clock.Now, Rand: w.nonces, Timeout: 10 * time.Second}
}

// message is a crossing message of one part from a member, of the body given as name and value.
func (w *world) message(t testing.TB, id, to, kind string, name string, value any, re string) []byte {
	t.Helper()
	ext := map[string]any{"accord": w.accord, "to": to}
	if re != "" {
		ext["re"] = re
	}
	m := map[string]any{
		"messageId": id, "role": mediator.RoleAgent, "extensions": []any{mediator.Extension},
		"metadata": map[string]any{mediator.Extension: ext},
		"parts": []any{map[string]any{name: value, "metadata": map[string]any{mediator.Extension: map[string]any{
			"kind": kind, "salt": hex.EncodeToString([]byte("a salt of sixteen " + id)[:16]),
		}}}},
	}
	b, _ := json.Marshal(m)
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func envelope(t testing.TB, seq int64, hash string, k signing.Key) []byte {
	t.Helper()
	e := signed(t, map[string]any{"seq": seq, "hash": hash}, signing.TypeEnvelope, k)
	b, _ := json.Marshal(e)
	c, _ := canonical.Form(b)
	return c
}

// give gives a text part from one member to another, at its next seq, and returns the reply.
func (w *world) give(t testing.TB, from, to, id, kind, text string) local.Reply {
	t.Helper()
	msg := w.message(t, id, key(t, to).Reference(), kind, "text", text, "")
	h, _ := canonical.TypedHash(msg)
	w.seqs[from]++
	rep, err := w.client(t, from).Call(local.OpGive, local.GiveArgs(msg, envelope(t, w.seqs[from], h, key(t, from))))
	if err != nil {
		t.Fatalf("give %s: %v", id, err)
	}
	if rep.Outcome == nil {
		w.seqs[from]--
	}
	return rep
}

func (w *world) call(t testing.TB, who, op string, args map[string]any) local.Reply {
	t.Helper()
	rep, err := w.client(t, who).Call(op, args)
	if err != nil {
		t.Fatalf("%s %s: %v", who, op, err)
	}
	return rep
}

func (w *world) receipts(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(w.journal, "receipts.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return b
}

// snapshot is every file of a directory and its bytes.
func snapshot(t testing.TB, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

func hashOf(msg []byte) string { h, _ := canonical.TypedHash(msg); return h }

type syncMutex = sync.Mutex

// askAndLogOther is the ask-and-log Accord with another operator, so another Accord id, under the
// same mediator.
func askAndLogOther(t testing.TB) []byte {
	t.Helper()
	set := askAndLog(t, "mediator")
	return bytes.Replace(set, []byte(key(t, "operator").Reference()), []byte(key(t, "other operator").Reference()), 1)
}
