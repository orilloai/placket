package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/local"
)

// TestTheWindowLogsTheDoorsOwnFailure prints a call event the door failed on itself: the line names
// the op, the door and its code, never a bare op that reads as a call with nothing to report.
func TestTheWindowLogsTheDoorsOwnFailure(t *testing.T) {
	t.Parallel()
	var errOut bytes.Buffer
	r := &run{stdin: strings.NewReader(""), stdout: io.Discard, stderr: &errOut, realOut: io.Discard, rep: newReport()}
	r.event(local.Event{Kind: local.EventCall, Op: local.OpResolve, Door: local.Internal, Release: -1}, "operator")
	if got, want := errOut.String(), "resolve door door-internal\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
