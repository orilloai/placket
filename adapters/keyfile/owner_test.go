package keyfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadRefusesAKeyAnotherUserOwns refuses, with key-mode, a key file of mode 0600 that the
// running user does not own. It reads as another uid by swapping getuid, so it does not run in
// parallel.
func TestReadRefusesAKeyAnotherUserOwns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.key")
	if err := os.WriteFile(path, []byte(strings.Repeat("0f", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		t.Fatalf("as its owner: %v", err)
	}
	real := getuid
	getuid = func() int { return real() + 1 }
	defer func() { getuid = real }()
	if _, err := Read(path); err == nil || err.Error() != "keyfile: key-mode" {
		t.Fatalf("as another user: got %v, want keyfile: key-mode", err)
	}
}
