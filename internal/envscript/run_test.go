package envscript

import (
	"fmt"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

type recordedExec struct {
	envVars map[string]string
	args    []string
}

func recordingExecutor(calls *[]recordedExec) EnvExecFunc {
	return func(envVars map[string]string, args []string) (int, error) {
		*calls = append(*calls, recordedExec{envVars: envVars, args: args})
		return 0, nil
	}
}

func TestRunEnvScriptsExecutesEachScript(t *testing.T) {
	var calls []recordedExec

	scripts := []config.ScriptEntry{
		{Path: "scripts/setup-db.sh"},
		{Path: "scripts/seed-data.sh"},
	}

	err := RunEnvScripts(scripts, "my-env", "container", recordingExecutor(&calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(calls))
	}

	if calls[0].args[1] != "scripts/setup-db.sh" {
		t.Errorf("expected first script setup-db.sh, got %v", calls[0].args)
	}
	if calls[1].args[1] != "scripts/seed-data.sh" {
		t.Errorf("expected second script seed-data.sh, got %v", calls[1].args)
	}
}

func TestRunEnvScriptsPassesISOLARIUMEnvVars(t *testing.T) {
	var calls []recordedExec

	scripts := []config.ScriptEntry{
		{Path: "scripts/setup.sh"},
	}

	err := RunEnvScripts(scripts, "my-env", "container", recordingExecutor(&calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := calls[0].envVars
	if env["ISOLARIUM_NAME"] != "my-env" {
		t.Errorf("expected ISOLARIUM_NAME=my-env, got %q", env["ISOLARIUM_NAME"])
	}
	if env["ISOLARIUM_TYPE"] != "container" {
		t.Errorf("expected ISOLARIUM_TYPE=container, got %q", env["ISOLARIUM_TYPE"])
	}
}

func TestRunEnvScriptsPassesDeclaredEnvVars(t *testing.T) {
	var calls []recordedExec

	t.Setenv("DB_PASSWORD", "secret123")

	scripts := []config.ScriptEntry{
		{Path: "scripts/setup.sh", Env: []string{"DB_PASSWORD"}},
	}

	err := RunEnvScripts(scripts, "my-env", "vm", recordingExecutor(&calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls[0].envVars["DB_PASSWORD"] != "secret123" {
		t.Errorf("expected DB_PASSWORD=secret123, got %q", calls[0].envVars["DB_PASSWORD"])
	}
}

func TestRunEnvScriptsFailsOnMissingEnvVars(t *testing.T) {
	var calls []recordedExec

	scripts := []config.ScriptEntry{
		{Path: "scripts/setup.sh", Env: []string{"NONEXISTENT_VAR"}},
	}

	err := RunEnvScripts(scripts, "my-env", "container", recordingExecutor(&calls))
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "NONEXISTENT_VAR") {
		t.Errorf("expected error to mention NONEXISTENT_VAR, got: %s", err.Error())
	}
	if len(calls) != 0 {
		t.Errorf("expected no exec calls, got %d", len(calls))
	}
}

func TestRunEnvScriptsPropagatesScriptFailure(t *testing.T) {
	failingExecutor := func(envVars map[string]string, args []string) (int, error) {
		return 1, fmt.Errorf("script failed with exit code 1")
	}

	scripts := []config.ScriptEntry{
		{Path: "scripts/setup.sh"},
	}

	err := RunEnvScripts(scripts, "my-env", "container", failingExecutor)
	if err == nil {
		t.Fatal("expected error when script fails")
	}
	if !strings.Contains(err.Error(), "setup.sh") {
		t.Errorf("expected error to mention script name, got: %s", err.Error())
	}
}

func TestRunEnvScriptsStopsOnFirstFailure(t *testing.T) {
	executionCount := 0
	failOnSecond := func(envVars map[string]string, args []string) (int, error) {
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

	err := RunEnvScripts(scripts, "my-env", "container", failOnSecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if executionCount != 2 {
		t.Errorf("expected 2 executions (stop on failure), got %d", executionCount)
	}
}

func TestRunEnvScriptsNoopWithEmptyScripts(t *testing.T) {
	var calls []recordedExec
	err := RunEnvScripts(nil, "my-env", "container", recordingExecutor(&calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 0 {
		t.Errorf("expected no exec calls, got %d", len(calls))
	}
}
