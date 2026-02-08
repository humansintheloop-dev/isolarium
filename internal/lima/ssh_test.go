package lima

import (
	"testing"
)

func TestBuildShellCommand(t *testing.T) {
	cmd := BuildShellCommand("isolarium")
	expected := []string{
		"limactl", "shell", "--tty", "isolarium",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(cmd), cmd)
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}
