package client_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/signing"
)

// worked is api/testdata/worked/, whose founding set and keys these tests run a window of.
const worked = "../../api/testdata/worked"

// A world is a window of the worked founding set and the key file of each party.
type world struct {
	accord, socket, dir string
	refs                map[string]string
}

func (w *world) key(name string) string { return filepath.Join(w.dir, name+".key") }

func newWorld(t *testing.T) *world {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(worked, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ks struct {
		Accord string
		Keys   map[string]struct{ Seed, Actor string }
	}
	if err := json.Unmarshal(b, &ks); err != nil {
		t.Fatal(err)
	}
	d, err := os.MkdirTemp("/tmp", "plc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	w := &world{accord: ks.Accord, socket: filepath.Join(d, "s"), dir: d, refs: map[string]string{}}
	for name, k := range ks.Keys {
		if err := os.WriteFile(w.key(name), []byte(k.Seed+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		w.refs[name] = k.Actor
	}
	openings, acceptances := workedFounding(t, worked)
	set, err := os.ReadFile(filepath.Join(worked, "founding.json"))
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := hex.DecodeString(ks.Keys["mediator"].Seed)
	mk, err := signing.NewKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- local.Run(ctx, local.Window{Key: mk, Founding: bytes.TrimSuffix(set, []byte{'\n'}), Openings: openings, Acceptances: acceptances, Socket: w.socket,
			Journal: filepath.Join(d, "j"), Now: time.Now, Idle: 20 * time.Millisecond, Ready: ready})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("the window did not start: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("the window ended with %v", err)
		}
	})
	return w
}

func (w *world) open(t *testing.T, name string) *client.Client {
	t.Helper()
	c, err := client.Open(client.Mediator{Accord: w.accord, Key: w.key(name), Socket: w.socket}, client.Options{Now: time.Now, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func note(text string) client.Body { return client.Body{Form: client.FormText, Text: text} }

// TestTheClientAndTheCommandLineTakeTurns gives a note with placket give and then with client.Give,
// on one key and one state directory: their envelopes carry seq 1 and 2. A client.Give while the
// state lock is held, as the command line's give holds it, waits until it is let go.
func TestTheClientAndTheCommandLineTakeTurns(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	var out, errb bytes.Buffer
	code := cli.Run(cli.Env{Now: time.Now, Rand: rand.Reader}, []string{"give", "--key", w.key("proposer"), "--socket", w.socket, "--accord", w.accord,
		"--to", w.refs["applier"], "--kind", "note", "--text", "One."}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("placket give: %d %s", code, errb.String())
	}
	c := w.open(t, "proposer")
	if rep, err := c.Give(context.Background(), client.Message{To: w.refs["applier"], Kind: "note", Body: note("Two.")}, false); err != nil || rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("client.Give: %+v, %v", rep, err)
	}
	envs, err := c.Dir().Envelopes()
	if err != nil || len(envs) != 2 || envs[0].Seq != 1 || envs[1].Seq != 2 {
		t.Fatalf("the envelopes: %+v, %v", envs, err)
	}
	held, err := state.Open(state.DefaultPath(w.key("proposer"), w.accord), w.refs["proposer"], &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Lock(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := w.open(t, "proposer").Give(context.Background(), client.Message{To: w.refs["applier"], Kind: "note", Body: note("Three.")}, false)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("a give while the lock is held ended: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	held.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the give did not go on once the lock was let go")
	}
	if envs, _ := c.Dir().Envelopes(); len(envs) != 3 || envs[2].Seq != 3 {
		t.Fatalf("the envelopes after: %+v", envs)
	}
}

// TestADeliveryIsPersistedBeforeItIsReturned fetches a note with client.Get: when it returns,
// delivered.jsonl holds its receipt. A process killed by the test hook between persisting and
// returning leaves the receipt in delivered.jsonl and, kept, the delivery in kept.jsonl, and the
// window's queue empty.
func TestADeliveryIsPersistedBeforeItIsReturned(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	p := w.open(t, "proposer")
	for _, text := range []string{"One.", "Two."} {
		if _, err := p.Give(context.Background(), client.Message{To: w.refs["applier"], Kind: "note", Body: note(text)}, false); err != nil {
			t.Fatal(err)
		}
	}
	a := w.open(t, "applier")
	g, err := a.Get(context.Background(), 0, false)
	if err != nil || g.Delivery == nil {
		t.Fatalf("client.Get: %+v, %v", g, err)
	}
	if d, err := a.Dir().Delivered(); err != nil || len(d) != 1 || !bytes.Equal(d[0], g.Delivery.Receipt) {
		t.Fatalf("delivered.jsonl: %d lines, %v", len(d), err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestKilledAfterPersisting$")
	cmd.Env = append(os.Environ(), "PLACKET_CLIENT_KILL="+strings.Join([]string{w.accord, w.key("applier"), w.socket}, "\n"))
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("the process was not killed: %v", err)
	}
	d, err := a.Dir().Delivered()
	if err != nil || len(d) != 2 {
		t.Fatalf("delivered.jsonl after the kill: %d lines, %v", len(d), err)
	}
	kept, err := a.Dir().Kept()
	if err != nil || len(kept) != 1 || !strings.Contains(string(kept[0]), `"text":"Two."`) {
		t.Fatalf("kept.jsonl after the kill: %q, %v", kept, err)
	}
	if g, err := a.Get(context.Background(), 0, false); err != nil || g.Delivery != nil {
		t.Fatalf("the queue after the kill: %+v, %v", g, err)
	}
}

// TestKilledAfterPersisting is the process TestADeliveryIsPersistedBeforeItIsReturned kills: it
// fetches, with keep, and kills itself once the delivery is persisted. Run on its own it does nothing.
func TestKilledAfterPersisting(t *testing.T) {
	v := os.Getenv("PLACKET_CLIENT_KILL")
	if v == "" {
		return
	}
	f := strings.Split(v, "\n")
	c, err := client.Open(client.Mediator{Accord: f[0], Key: f[1], Socket: f[2]}, client.Options{Now: time.Now, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	client.SetPersisted(func() { _ = syscall.Kill(os.Getpid(), syscall.SIGKILL) })
	_, _ = c.Get(context.Background(), 0, true)
	t.Fatal("not killed: " + strconv.Itoa(os.Getpid()))
}

// workedFounding reads the worked Accord's openings, each under its actor, and its acceptances, so a
// window of the worked set is founded at once (setup-contract-15).
func workedFounding(t *testing.T, dir string) (map[string][]byte, []byte) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.opening.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no worked openings: %v", err)
	}
	openings := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		openings["ed25519:"+strings.TrimSuffix(filepath.Base(f), ".opening.json")] = bytes.TrimSuffix(b, []byte{'\n'})
	}
	acc, err := os.ReadFile(filepath.Join(dir, "acceptances.json"))
	if err != nil {
		t.Fatal(err)
	}
	return openings, bytes.TrimSuffix(acc, []byte{'\n'})
}
