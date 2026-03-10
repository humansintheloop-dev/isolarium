package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPidConfigParsesValidYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `isolarium:
  container:
    isolation_scripts:
      - path: scripts/container/install-go.sh
      - path: scripts/container/install-codescene.sh
        env:
          - CS_ACCESS_TOKEN
          - CS_ACE_ACCESS_TOKEN
    host_scripts:
      - path: scripts/setup-mcp.sh
        env:
          - CS_ACCESS_TOKEN
  vm:
    isolation_scripts:
      - path: scripts/vm/install-go.sh
`
	writeFile(t, dir, "pid.yaml", yaml)

	cfg, err := LoadPidConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	assertScriptCount(t, cfg.Container.IsolationScripts, 2, "container.isolation_scripts")
	assertScriptPath(t, cfg.Container.IsolationScripts[0], "scripts/container/install-go.sh")
	assertScriptPath(t, cfg.Container.IsolationScripts[1], "scripts/container/install-codescene.sh")
	assertEnvVars(t, cfg.Container.IsolationScripts[1], []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"})

	assertScriptCount(t, cfg.Container.HostScripts, 1, "container.host_scripts")
	assertScriptPath(t, cfg.Container.HostScripts[0], "scripts/setup-mcp.sh")
	assertEnvVars(t, cfg.Container.HostScripts[0], []string{"CS_ACCESS_TOKEN"})

	assertScriptCount(t, cfg.VM.IsolationScripts, 1, "vm.isolation_scripts")
	assertScriptPath(t, cfg.VM.IsolationScripts[0], "scripts/vm/install-go.sh")
	assertScriptCount(t, cfg.VM.HostScripts, 0, "vm.host_scripts")
}

func TestLoadPidConfigReturnsNilWhenFileAbsent(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadPidConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

func TestLoadPidConfigRejectsInvalidScriptEntries(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		errContains string
	}{
		{
			name: "missing path field",
			yaml: `isolarium:
  container:
    isolation_scripts:
      - env:
          - SOME_VAR
`,
			errContains: "path",
		},
		{
			name: "path traversal above project root",
			yaml: `isolarium:
  container:
    isolation_scripts:
      - path: ../../../etc/passwd
`,
			errContains: "escapes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "pid.yaml", tt.yaml)

			_, err := LoadPidConfig(dir)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.errContains)
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("expected error to contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

func assertScriptCount(t *testing.T, scripts []ScriptEntry, expected int, label string) {
	t.Helper()
	if len(scripts) != expected {
		t.Errorf("expected %d %s, got %d", expected, label, len(scripts))
	}
}

func assertScriptPath(t *testing.T, script ScriptEntry, expected string) {
	t.Helper()
	if script.Path != expected {
		t.Errorf("expected path %q, got %q", expected, script.Path)
	}
}

func assertEnvVars(t *testing.T, script ScriptEntry, expected []string) {
	t.Helper()
	if len(script.Env) != len(expected) {
		t.Errorf("expected %d env vars, got %d", len(expected), len(script.Env))
		return
	}
	for i, v := range expected {
		if script.Env[i] != v {
			t.Errorf("expected env[%d] = %q, got %q", i, v, script.Env[i])
		}
	}
}
