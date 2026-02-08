package lima

import (
	"testing"
)

func TestBuildExecCommand_SimpleEcho(t *testing.T) {
	cmd := BuildExecCommand("isolarium", "~/repo", []string{"echo", "hello"})
	expected := []string{
		"limactl", "shell", "isolarium", "--workdir", "~/repo", "--", "echo", "hello",
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

func TestBuildExecCommand_MultipleArgs(t *testing.T) {
	cmd := BuildExecCommand("isolarium", "~/repo", []string{"git", "status", "--short"})
	expected := []string{
		"limactl", "shell", "isolarium", "--workdir", "~/repo", "--", "git", "status", "--short",
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

func TestBuildExecCommand_Pwd(t *testing.T) {
	cmd := BuildExecCommand("isolarium", "~/repo", []string{"pwd"})
	expected := []string{
		"limactl", "shell", "isolarium", "--workdir", "~/repo", "--", "pwd",
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

func TestBuildInteractiveExecCommand_IncludesTTYFlag(t *testing.T) {
	cmd := BuildInteractiveExecCommand("isolarium", "~/repo", []string{"claude"})
	expected := []string{
		"limactl", "shell", "--tty", "isolarium", "--workdir", "~/repo", "--", "claude",
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

func TestBuildInteractiveExecCommand_MultipleArgs(t *testing.T) {
	cmd := BuildInteractiveExecCommand("isolarium", "~/repo", []string{"claude", "-p", "hello"})
	expected := []string{
		"limactl", "shell", "--tty", "isolarium", "--workdir", "~/repo", "--", "claude", "-p", "hello",
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
