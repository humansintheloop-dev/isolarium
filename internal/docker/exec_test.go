package docker

import (
	"testing"
)

func TestBuildExecCommand_SimpleCommand(t *testing.T) {
	cmd := BuildExecCommand("my-container", nil, []string{"echo", "hello"})
	expected := []string{
		"docker", "exec", "my-container", "echo", "hello",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildExecCommand_WithEnvVars(t *testing.T) {
	envVars := map[string]string{"GH_TOKEN": "ghs_abc123"}
	cmd := BuildExecCommand("my-container", envVars, []string{"echo", "hello"})
	expected := []string{
		"docker", "exec", "-e", "GH_TOKEN=ghs_abc123", "my-container", "echo", "hello",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildExecCommand_WithEmptyEnvVars(t *testing.T) {
	cmd := BuildExecCommand("my-container", map[string]string{}, []string{"echo", "hello"})
	expected := []string{
		"docker", "exec", "my-container", "echo", "hello",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildExecCommand_WithMultipleEnvVars(t *testing.T) {
	envVars := map[string]string{"GH_TOKEN": "ghs_abc123", "API_KEY": "key456"}
	cmd := BuildExecCommand("my-container", envVars, []string{"echo", "hello"})
	expected := []string{
		"docker", "exec", "-e", "API_KEY=key456", "-e", "GH_TOKEN=ghs_abc123", "my-container", "echo", "hello",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildInteractiveExecCommand_IncludesITFlags(t *testing.T) {
	cmd := BuildInteractiveExecCommand("my-container", nil, []string{"bash"})
	expected := []string{
		"docker", "exec", "-it", "my-container", "bash",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildInteractiveExecCommand_WithEnvVars(t *testing.T) {
	envVars := map[string]string{"GH_TOKEN": "ghs_abc123"}
	cmd := BuildInteractiveExecCommand("my-container", envVars, []string{"bash"})
	expected := []string{
		"docker", "exec", "-it", "-e", "GH_TOKEN=ghs_abc123", "my-container", "bash",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildInteractiveExecCommand_MultipleArgs(t *testing.T) {
	cmd := BuildInteractiveExecCommand("my-container", nil, []string{"claude", "-p", "hello"})
	expected := []string{
		"docker", "exec", "-it", "my-container", "claude", "-p", "hello",
	}
	assertArgsEqual(t, expected, cmd)
}

func assertArgsEqual(t *testing.T, expected, actual []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(actual), actual)
	}
	for i, arg := range expected {
		if actual[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, actual[i])
		}
	}
}
