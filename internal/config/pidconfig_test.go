package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPidConfigParsesLifecycleGroupedYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `isolarium:
  container:
    create:
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
    run:
      env:
        - CS_ACCESS_TOKEN
        - CS_ACE_ACCESS_TOKEN
  vm:
    create:
      isolation_scripts:
        - path: scripts/vm/install-go.sh
    run:
      env:
        - CS_ACCESS_TOKEN
`
	writeFile(t, dir, "pid.yaml", yaml)

	cfg, err := LoadPidConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	assertScriptCount(t, cfg.Container.Create.IsolationScripts, 2, "container.create.isolation_scripts")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[0], "scripts/container/install-go.sh")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[1], "scripts/container/install-codescene.sh")
	assertEnvVars(t, cfg.Container.Create.IsolationScripts[1], []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"})

	assertScriptCount(t, cfg.Container.Create.HostScripts, 1, "container.create.host_scripts")
	assertScriptPath(t, cfg.Container.Create.HostScripts[0], "scripts/setup-mcp.sh")
	assertEnvVars(t, cfg.Container.Create.HostScripts[0], []string{"CS_ACCESS_TOKEN"})

	assertRunEnv(t, cfg.Container.Run.Env, []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"}, "container.run.env")

	assertScriptCount(t, cfg.VM.Create.IsolationScripts, 1, "vm.create.isolation_scripts")
	assertScriptPath(t, cfg.VM.Create.IsolationScripts[0], "scripts/vm/install-go.sh")
	assertScriptCount(t, cfg.VM.Create.HostScripts, 0, "vm.create.host_scripts")

	assertRunEnv(t, cfg.VM.Run.Env, []string{"CS_ACCESS_TOKEN"}, "vm.run.env")
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

func TestLoadRepoPidYAML(t *testing.T) {
	repoRoot := findRepoRoot(t)

	cfg, err := LoadPidConfig(repoRoot)
	if err != nil {
		t.Fatalf("failed to parse repo pid.yaml: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected repo pid.yaml to exist and parse")
	}

	assertScriptCount(t, cfg.Container.Create.IsolationScripts, 4, "container.create.isolation_scripts")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[0], "scripts/container/install-go.sh")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[1], "scripts/container/install-linters.sh")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[2], "scripts/container/install-pre-commit.sh")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[3], "scripts/container/install-codescene.sh")
	assertEnvVars(t, cfg.Container.Create.IsolationScripts[3], []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"})

	assertScriptCount(t, cfg.VM.Create.IsolationScripts, 4, "vm.create.isolation_scripts")
	assertScriptPath(t, cfg.VM.Create.IsolationScripts[0], "scripts/vm/install-go.sh")
	assertScriptPath(t, cfg.VM.Create.IsolationScripts[1], "scripts/vm/install-linters.sh")
	assertScriptPath(t, cfg.VM.Create.IsolationScripts[2], "scripts/vm/install-pre-commit.sh")
	assertScriptPath(t, cfg.VM.Create.IsolationScripts[3], "scripts/vm/install-codescene.sh")
	assertEnvVars(t, cfg.VM.Create.IsolationScripts[3], []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"})
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}

func TestLoadPidConfigBackwardCompatWithFlatStructure(t *testing.T) {
	dir := t.TempDir()
	yaml := `isolarium:
  container:
    isolation_scripts:
      - path: scripts/container/install-go.sh
    host_scripts:
      - path: scripts/setup-mcp.sh
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

	assertScriptCount(t, cfg.Container.Create.IsolationScripts, 1, "container.create.isolation_scripts")
	assertScriptPath(t, cfg.Container.Create.IsolationScripts[0], "scripts/container/install-go.sh")

	assertScriptCount(t, cfg.Container.Create.HostScripts, 1, "container.create.host_scripts")
	assertScriptPath(t, cfg.Container.Create.HostScripts[0], "scripts/setup-mcp.sh")

	assertScriptCount(t, cfg.VM.Create.IsolationScripts, 1, "vm.create.isolation_scripts")
	assertScriptPath(t, cfg.VM.Create.IsolationScripts[0], "scripts/vm/install-go.sh")
}

func assertRunEnv(t *testing.T, actual, expected []string, label string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("%s: expected %d env vars, got %d", label, len(expected), len(actual))
		return
	}
	for i, v := range expected {
		if actual[i] != v {
			t.Errorf("%s: expected env[%d] = %q, got %q", label, i, v, actual[i])
		}
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
