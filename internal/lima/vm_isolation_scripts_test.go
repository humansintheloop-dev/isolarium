package lima

import (
	"fmt"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

type recordedCommand struct {
	vm      string
	workdir string
	envVars map[string]string
	args    []string
}

func recordingExecutor(commands *[]recordedCommand) func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
	return func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
		*commands = append(*commands, recordedCommand{vm: vm, workdir: workdir, envVars: envVars, args: args})
		return 0, nil
	}
}

func TestRunVMIsolationScriptsExecutesEachScriptViaLimactlShell(t *testing.T) {
	var commands []recordedCommand

	scripts := []config.ScriptEntry{
		{Path: "scripts/install-go.sh"},
		{Path: "scripts/install-linters.sh"},
	}

	err := RunVMIsolationScripts(scripts, "test-vm", "/vm/repo", recordingExecutor(&commands))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}

	expected := recordedCommand{vm: "test-vm", workdir: "/vm/repo", args: []string{"bash", "scripts/install-go.sh"}}
	assertCommandMatches(t, commands[0], expected)
	expected.args = []string{"bash", "scripts/install-linters.sh"}
	assertCommandMatches(t, commands[1], expected)
}

func TestRunVMIsolationScriptsPassesEnvVars(t *testing.T) {
	var commands []recordedCommand

	t.Setenv("CS_ACCESS_TOKEN", "token123")
	t.Setenv("CS_ACE_ACCESS_TOKEN", "ace456")

	scripts := []config.ScriptEntry{
		{
			Path: "scripts/install-codescene.sh",
			Env:  []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"},
		},
	}

	err := RunVMIsolationScripts(scripts, "test-vm", "/vm/repo", recordingExecutor(&commands))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	env := commands[0].envVars
	if env["CS_ACCESS_TOKEN"] != "token123" {
		t.Errorf("expected CS_ACCESS_TOKEN=token123, got %q", env["CS_ACCESS_TOKEN"])
	}
	if env["CS_ACE_ACCESS_TOKEN"] != "ace456" {
		t.Errorf("expected CS_ACE_ACCESS_TOKEN=ace456, got %q", env["CS_ACE_ACCESS_TOKEN"])
	}
}

func TestRunVMIsolationScriptsFailsOnMissingEnvVars(t *testing.T) {
	var commands []recordedCommand

	scripts := []config.ScriptEntry{
		{
			Path: "scripts/install-codescene.sh",
			Env:  []string{"MISSING_VAR_1", "MISSING_VAR_2"},
		},
	}

	err := RunVMIsolationScripts(scripts, "test-vm", "/vm/repo", recordingExecutor(&commands))
	if err == nil {
		t.Fatal("expected error for missing env vars")
	}

	if !strings.Contains(err.Error(), "MISSING_VAR_1") {
		t.Errorf("expected error to mention MISSING_VAR_1, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "MISSING_VAR_2") {
		t.Errorf("expected error to mention MISSING_VAR_2, got: %s", err.Error())
	}

	if len(commands) != 0 {
		t.Errorf("expected no commands to be executed, got %d", len(commands))
	}
}

func TestRunVMIsolationScriptsPropagatesScriptFailure(t *testing.T) {
	failingExecutor := func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
		return 1, fmt.Errorf("script failed with exit code 1")
	}

	scripts := []config.ScriptEntry{
		{Path: "scripts/install-go.sh"},
	}

	err := RunVMIsolationScripts(scripts, "test-vm", "/vm/repo", failingExecutor)
	if err == nil {
		t.Fatal("expected error when script fails")
	}

	if !strings.Contains(err.Error(), "install-go.sh") {
		t.Errorf("expected error to mention script name, got: %s", err.Error())
	}
}

func TestRunVMIsolationScriptsStopsOnFirstFailure(t *testing.T) {
	executionCount := 0
	failOnSecond := func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
		executionCount++
		if executionCount == 2 {
			return 1, fmt.Errorf("script failed")
		}
		return 0, nil
	}

	scripts := []config.ScriptEntry{
		{Path: "scripts/first.sh"},
		{Path: "scripts/second.sh"},
		{Path: "scripts/third.sh"},
	}

	err := RunVMIsolationScripts(scripts, "test-vm", "/vm/repo", failOnSecond)
	if err == nil {
		t.Fatal("expected error")
	}

	if executionCount != 2 {
		t.Errorf("expected 2 executions (stop on failure), got %d", executionCount)
	}
}

func TestRunVMIsolationScriptsNoopWithEmptyScripts(t *testing.T) {
	var commands []recordedCommand
	err := RunVMIsolationScripts(nil, "test-vm", "/vm/repo", recordingExecutor(&commands))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commands) != 0 {
		t.Errorf("expected no commands, got %d", len(commands))
	}
}

func TestRunVMIsolationScriptsPassesISOLARIUMEnvVars(t *testing.T) {
	var commands []recordedCommand

	scripts := []config.ScriptEntry{
		{Path: "scripts/setup.sh"},
	}

	err := RunVMIsolationScripts(scripts, "my-vm", "/vm/repo", recordingExecutor(&commands))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := commands[0].envVars
	if env["ISOLARIUM_NAME"] != "my-vm" {
		t.Errorf("expected ISOLARIUM_NAME=my-vm, got %q", env["ISOLARIUM_NAME"])
	}
	if env["ISOLARIUM_TYPE"] != "vm" {
		t.Errorf("expected ISOLARIUM_TYPE=vm, got %q", env["ISOLARIUM_TYPE"])
	}
}

func assertCommandMatches(t *testing.T, actual, expected recordedCommand) {
	t.Helper()
	if actual.vm != expected.vm {
		t.Errorf("expected vm %q, got %q", expected.vm, actual.vm)
	}
	if actual.workdir != expected.workdir {
		t.Errorf("expected workdir %q, got %q", expected.workdir, actual.workdir)
	}
	if len(actual.args) != len(expected.args) {
		t.Fatalf("expected args %v, got %v", expected.args, actual.args)
	}
	for i, arg := range expected.args {
		if actual.args[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, actual.args[i])
		}
	}
}
