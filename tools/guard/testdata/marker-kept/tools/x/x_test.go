package x

import (
	"os"
	"os/exec"
	"testing"
)

func keep(t *testing.T) {
	_ = os.Getenv("PLACKET_IN_CI")
	t.Setenv("PLACKET_IN_CI", "1")
	cmd := exec.Command("make", "ci")
	cmd.Env = append(os.Environ(), "PLACKET_IN_CI=1")
	if cmd.Env == nil {
		t.Fatal("no env") // os.Unsetenv("PLACKET_IN_CI") in a comment is not read
	}
}
