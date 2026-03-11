package hostscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

func TestRunHostScriptsExecutesScriptWithIsolationEnvVars(t *testing.T) {
	workDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "marker.txt")

	scriptContent := "#!/bin/sh\necho \"NAME=$ISOLARIUM_NAME TYPE=$ISOLARIUM_TYPE\" > " + markerFile + "\n"
	writeScript(t, workDir, "setup.sh", scriptContent)

	scripts := []config.ScriptEntry{{Path: "setup.sh"}}

	err := RunHostScripts(scripts, workDir, "my-container", "container")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("marker file not created: %v", err)
	}

	expected := "NAME=my-container TYPE=container"
	if !strings.Contains(string(data), expected) {
		t.Errorf("expected marker to contain %q, got %q", expected, string(data))
	}
}

func TestRunHostScriptsExecutesInDeclaredOrder(t *testing.T) {
	workDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "order.txt")

	writeScript(t, workDir, "first.sh", "#!/bin/sh\necho first >> "+markerFile+"\n")
	writeScript(t, workDir, "second.sh", "#!/bin/sh\necho second >> "+markerFile+"\n")
	writeScript(t, workDir, "third.sh", "#!/bin/sh\necho third >> "+markerFile+"\n")

	scripts := []config.ScriptEntry{
		{Path: "first.sh"},
		{Path: "second.sh"},
		{Path: "third.sh"},
	}

	err := RunHostScripts(scripts, workDir, "test-env", "container")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("marker file not created: %v", err)
	}

	expected := "first\nsecond\nthird\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

func TestRunHostScriptsReturnsErrorOnScriptFailure(t *testing.T) {
	workDir := t.TempDir()

	writeScript(t, workDir, "fail.sh", "#!/bin/sh\nexit 1\n")

	scripts := []config.ScriptEntry{{Path: "fail.sh"}}

	err := RunHostScripts(scripts, workDir, "test-env", "container")
	if err == nil {
		t.Fatal("expected error when script fails")
	}

	if !strings.Contains(err.Error(), "fail.sh") {
		t.Errorf("expected error to mention script name, got: %v", err)
	}
}

func TestRunHostScriptsFailsWhenDeclaredEnvVarMissing(t *testing.T) {
	workDir := t.TempDir()

	writeScript(t, workDir, "needs-env.sh", "#!/bin/sh\necho ok\n")

	scripts := []config.ScriptEntry{
		{Path: "needs-env.sh", Env: []string{"MISSING_HOST_VAR_XYZ"}},
	}

	err := RunHostScripts(scripts, workDir, "test-env", "container")
	if err == nil {
		t.Fatal("expected error for missing env var")
	}

	if !strings.Contains(err.Error(), "MISSING_HOST_VAR_XYZ") {
		t.Errorf("expected error to mention MISSING_HOST_VAR_XYZ, got: %v", err)
	}
}

func TestRunHostScriptsPassesDeclaredEnvVarsToScript(t *testing.T) {
	workDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "env-marker.txt")

	scriptContent := "#!/bin/sh\necho \"TOKEN=$MY_HOST_TOKEN\" > " + markerFile + "\n"
	writeScript(t, workDir, "use-env.sh", scriptContent)

	t.Setenv("MY_HOST_TOKEN", "secret-123")

	scripts := []config.ScriptEntry{
		{Path: "use-env.sh", Env: []string{"MY_HOST_TOKEN"}},
	}

	err := RunHostScripts(scripts, workDir, "test-env", "container")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("marker file not created: %v", err)
	}

	if !strings.Contains(string(data), "TOKEN=secret-123") {
		t.Errorf("expected marker to contain TOKEN=secret-123, got %q", string(data))
	}
}

func TestRunHostScriptsWithNoScriptsSucceeds(t *testing.T) {
	err := RunHostScripts(nil, t.TempDir(), "test-env", "container")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeScript(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write script %s: %v", name, err)
	}
}
