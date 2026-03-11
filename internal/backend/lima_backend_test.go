package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type vmTestFixture struct {
	workDir string
	t       *testing.T
}

func newVMTestFixture(t *testing.T) vmTestFixture {
	t.Helper()
	return vmTestFixture{workDir: t.TempDir(), t: t}
}

func (f vmTestFixture) writeScript(scriptContent []byte) {
	f.t.Helper()
	scriptsDir := filepath.Join(f.workDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "setup.sh"), scriptContent, 0755); err != nil {
		f.t.Fatal(err)
	}
}

func (f vmTestFixture) writePidYaml(yaml []byte) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.workDir, "pid.yaml"), yaml, 0644); err != nil {
		f.t.Fatal(err)
	}
}

func (f vmTestFixture) readMarkerFile(name string) string {
	f.t.Helper()
	content, err := os.ReadFile(filepath.Join(f.workDir, name))
	if err != nil {
		f.t.Fatalf("host script did not create marker file: %v", err)
	}
	return string(content)
}

func noopCreateVM(_ string) error { return nil }

func TestLimaBackendCreateRunsHostScriptsAfterVMCreate(t *testing.T) {
	fix := newVMTestFixture(t)
	markerFile := fix.workDir + "/host-script-ran"

	fix.writeScript([]byte("#!/bin/bash\necho \"NAME=$ISOLARIUM_NAME TYPE=$ISOLARIUM_TYPE\" > " + markerFile + "\n"))
	fix.writePidYaml([]byte(`isolarium:
  vm:
    host_scripts:
      - path: scripts/setup.sh
`))

	vmCreated := false
	b := &LimaBackend{
		CreateVMFunc: func(name string) error {
			vmCreated = true
			return nil
		},
	}

	err := b.Create("test-vm", CreateOptions{WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !vmCreated {
		t.Fatal("expected CreateVMFunc to be called")
	}

	output := fix.readMarkerFile("host-script-ran")
	if !strings.Contains(output, "NAME=test-vm") {
		t.Errorf("expected ISOLARIUM_NAME=test-vm, got: %s", output)
	}
	if !strings.Contains(output, "TYPE=vm") {
		t.Errorf("expected ISOLARIUM_TYPE=vm, got: %s", output)
	}
}

func TestLimaBackendCreateWithHostScriptDeclaredEnvVars(t *testing.T) {
	fix := newVMTestFixture(t)
	markerFile := fix.workDir + "/env-marker"

	fix.writeScript([]byte("#!/bin/bash\necho \"TOKEN=$MY_SECRET\" > " + markerFile + "\n"))
	fix.writePidYaml([]byte(`isolarium:
  vm:
    host_scripts:
      - path: scripts/setup.sh
        env:
          - MY_SECRET
`))

	t.Setenv("MY_SECRET", "super-secret-value")

	b := &LimaBackend{CreateVMFunc: noopCreateVM}

	err := b.Create("test-vm", CreateOptions{WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := fix.readMarkerFile("env-marker")
	if !strings.Contains(output, "TOKEN=super-secret-value") {
		t.Errorf("expected MY_SECRET to be passed, got: %s", output)
	}
}

func TestLimaBackendCreateSucceedsWithoutPidYaml(t *testing.T) {
	fix := newVMTestFixture(t)

	b := &LimaBackend{CreateVMFunc: noopCreateVM}

	err := b.Create("test-vm", CreateOptions{WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
