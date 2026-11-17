package local

import (
	"net"
	"os"
	"syscall"
	"testing"
)

// timeoutError is a net.Error that timed out.
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestAcceptRetriesOnlyWhatMayPass tries an accept again only when the process or the system is out
// of file descriptors, or the accept timed out; a closed listener or any other failure ends the
// window. The pause doubles from 10 ms to 1 s.
func TestAcceptRetriesOnlyWhatMayPass(t *testing.T) {
	t.Parallel()
	op := func(err error) error {
		return &net.OpError{Op: "accept", Net: "unix", Err: os.NewSyscallError("accept4", err)}
	}
	for _, c := range []struct {
		err  error
		want bool
	}{
		{op(syscall.EMFILE), true},
		{op(syscall.ENFILE), true},
		{&net.OpError{Op: "accept", Net: "unix", Err: timeoutError{}}, true},
		{op(syscall.ECONNRESET), false},
		{op(syscall.EINVAL), false},
		{net.ErrClosed, false},
	} {
		if got := temporary(c.err); got != c.want {
			t.Fatalf("%v: got %v, want %v", c.err, got, c.want)
		}
	}
	if minAcceptPause.Milliseconds() != 10 || maxAcceptPause.Milliseconds() != 1000 {
		t.Fatalf("pauses %v to %v", minAcceptPause, maxAcceptPause)
	}
}

// TestAReplyThatCannotBeEncodedIsTheDoors reads the reply the door sends when it cannot encode one:
// a refusal under the door member, with door-internal.
func TestAReplyThatCannotBeEncodedIsTheDoors(t *testing.T) {
	t.Parallel()
	if internalReply != `{"door":"door-internal","ok":false}` {
		t.Fatalf("got %s", internalReply)
	}
	r, ok := ParseReply([]byte(internalReply))
	if !ok || r.Door != Internal || r.Error != "" {
		t.Fatalf("read as %v, door %q, error %q", ok, r.Door, r.Error)
	}
}
