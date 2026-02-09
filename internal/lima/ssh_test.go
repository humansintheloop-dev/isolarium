package lima

import (
	"testing"
)

func TestBuildShellCommand(t *testing.T) {
	cmd := BuildShellCommand("isolarium", nil)
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

func TestBuildShellCommandWithEnvVars(t *testing.T) {
	envVars := map[string]string{
		"GH_TOKEN":  "tok123",
		"GIT_TOKEN": "tok123",
	}
	cmd := BuildShellCommand("isolarium", envVars)
	expected := []string{
		"limactl", "shell", "--tty", "isolarium", "--",
		"env", "GH_TOKEN=tok123", "GIT_TOKEN=tok123",
		"bash", "-il",
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

func TestBuildShellCommandWithEmptyEnvVars(t *testing.T) {
	cmd := BuildShellCommand("isolarium", map[string]string{})
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
