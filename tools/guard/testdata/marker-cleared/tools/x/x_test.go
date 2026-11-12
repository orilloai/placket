package x

import (
	"os"
	"os/exec"
	"testing"
)

func clear(t *testing.T) {
	os.Unsetenv("PLACKET_IN_CI")
	os.Clearenv()
	t.Setenv("PLACKET_IN_CI", "")
	cmd := exec.Command("make", "ci")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	_ = exec.Command("env", "-i", "make", "ci")
	_ = exec.Command("env", "-u", "PLACKET_IN_CI", "make", "ci")
	_ = exec.Cmd{Path: "make", Env: nil}
	_ = []string{"PLACKET_IN_CI=", "X=1"}
}
