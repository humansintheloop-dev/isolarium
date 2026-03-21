package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/lima"
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

func stubVMHomeDir(_ string) (string, error) { return "/vm", nil }

func TestLimaBackendCreateRunsHostScriptsAfterVMCreate(t *testing.T) {
	fix := newVMTestFixture(t)
	markerFile := fix.workDir + "/host-script-ran"

	fix.writeScript([]byte("#!/bin/bash\necho \"NAME=$ISOLARIUM_NAME TYPE=$ISOLARIUM_TYPE\" > " + markerFile + "\n"))
	fix.writePidYaml([]byte(`isolarium:
  vm:
    create:
      post_creation_scripts:
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

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
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
    create:
      post_creation_scripts:
        host_scripts:
          - path: scripts/setup.sh
            env:
              - MY_SECRET
`))

	t.Setenv("MY_SECRET", "super-secret-value")

	b := &LimaBackend{CreateVMFunc: noopCreateVM}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := fix.readMarkerFile("env-marker")
	if !strings.Contains(output, "TOKEN=super-secret-value") {
		t.Errorf("expected MY_SECRET to be passed, got: %s", output)
	}
}

func TestLimaBackendCreateRunsEnvScriptsInsideVM(t *testing.T) {
	fix := newVMTestFixture(t)
	fix.writeScript([]byte("#!/bin/bash\necho hi"))
	fix.writePidYaml([]byte(`isolarium:
  vm:
    create:
      post_creation_scripts:
        env_scripts:
          - path: scripts/setup.sh
`))

	var executed []recordedVMExec
	b := &LimaBackend{
		CreateVMFunc:  noopCreateVM,
		VMHomeDirFunc: stubVMHomeDir,
		VMExecFunc: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
			executed = append(executed, recordedVMExec{vm: vm, workdir: workdir, envVars: envVars, args: args})
			return 0, nil
		},
	}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executed) != 1 {
		t.Fatalf("expected 1 VM exec call, got %d", len(executed))
	}

	if executed[0].vm != "test-vm" {
		t.Errorf("expected vm test-vm, got %q", executed[0].vm)
	}
	if executed[0].workdir != "/vm/repo" {
		t.Errorf("expected workdir /vm/repo, got %q", executed[0].workdir)
	}
	if executed[0].args[1] != "scripts/setup.sh" {
		t.Errorf("expected script setup.sh, got %v", executed[0].args)
	}
	if executed[0].envVars["ISOLARIUM_NAME"] != "test-vm" {
		t.Errorf("expected ISOLARIUM_NAME=test-vm, got %q", executed[0].envVars["ISOLARIUM_NAME"])
	}
	if executed[0].envVars["ISOLARIUM_TYPE"] != "vm" {
		t.Errorf("expected ISOLARIUM_TYPE=vm, got %q", executed[0].envVars["ISOLARIUM_TYPE"])
	}
}

func TestLimaBackendCreatePassesEnvVarsToEnvScripts(t *testing.T) {
	fix := newVMTestFixture(t)
	fix.writePidYaml([]byte(`isolarium:
  vm:
    create:
      post_creation_scripts:
        env_scripts:
          - path: scripts/install-plugin.sh
          - path: scripts/add-codescene-mcp.sh
            env:
              - CS_ACCESS_TOKEN
              - CS_ACE_ACCESS_TOKEN
`))

	t.Setenv("CS_ACCESS_TOKEN", "token123")
	t.Setenv("CS_ACE_ACCESS_TOKEN", "ace456")

	var executed []recordedVMExec
	b := &LimaBackend{
		CreateVMFunc:  noopCreateVM,
		VMHomeDirFunc: stubVMHomeDir,
		VMExecFunc: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
			executed = append(executed, recordedVMExec{vm: vm, workdir: workdir, envVars: envVars, args: args})
			return 0, nil
		},
	}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executed) != 2 {
		t.Fatalf("expected 2 env script calls, got %d", len(executed))
	}

	if executed[0].envVars["CS_ACCESS_TOKEN"] != "" {
		t.Errorf("install-plugin.sh should not receive CS_ACCESS_TOKEN, got %q", executed[0].envVars["CS_ACCESS_TOKEN"])
	}

	if executed[1].envVars["CS_ACCESS_TOKEN"] != "token123" {
		t.Errorf("expected CS_ACCESS_TOKEN=token123, got %q", executed[1].envVars["CS_ACCESS_TOKEN"])
	}
	if executed[1].envVars["CS_ACE_ACCESS_TOKEN"] != "ace456" {
		t.Errorf("expected CS_ACE_ACCESS_TOKEN=ace456, got %q", executed[1].envVars["CS_ACE_ACCESS_TOKEN"])
	}
}

func TestLimaBackendCreateRunsEnvScriptsAfterHostScripts(t *testing.T) {
	fix := newVMTestFixture(t)
	markerFile := fix.workDir + "/host-ran"

	fix.writeScript([]byte("#!/bin/bash\necho ran > " + markerFile + "\n"))
	fix.writePidYaml([]byte(`isolarium:
  vm:
    create:
      post_creation_scripts:
        host_scripts:
          - path: scripts/setup.sh
        env_scripts:
          - path: scripts/env-setup.sh
`))

	var envExecCalls []recordedVMExec
	b := &LimaBackend{
		CreateVMFunc:  noopCreateVM,
		VMHomeDirFunc: stubVMHomeDir,
		VMExecFunc: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
			envExecCalls = append(envExecCalls, recordedVMExec{vm: vm, workdir: workdir, envVars: envVars, args: args})
			return 0, nil
		},
	}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fix.readMarkerFile("host-ran")

	if len(envExecCalls) != 1 {
		t.Fatalf("expected 1 env exec call, got %d", len(envExecCalls))
	}
	if envExecCalls[0].args[1] != "scripts/env-setup.sh" {
		t.Errorf("expected env script env-setup.sh, got %v", envExecCalls[0].args)
	}
}

func TestLimaBackendCreateSucceedsWithoutPidYaml(t *testing.T) {
	fix := newVMTestFixture(t)

	b := &LimaBackend{CreateVMFunc: noopCreateVM}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recordedVMExec struct {
	vm      string
	workdir string
	envVars map[string]string
	args    []string
}

func TestLimaBackendCreateRunsVMIsolationScripts(t *testing.T) {
	fix := newVMTestFixture(t)
	fix.writePidYaml([]byte(`isolarium:
  vm:
    create:
      creation_scripts:
        - path: scripts/install-go.sh
        - path: scripts/install-linters.sh
`))

	var executed []recordedVMExec
	b := &LimaBackend{
		CreateVMFunc:  noopCreateVM,
		VMHomeDirFunc: stubVMHomeDir,
		VMExecFunc: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
			executed = append(executed, recordedVMExec{vm: vm, workdir: workdir, envVars: envVars, args: args})
			return 0, nil
		},
	}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executed) != 2 {
		t.Fatalf("expected 2 VM exec calls, got %d", len(executed))
	}

	if executed[0].args[1] != "scripts/install-go.sh" {
		t.Errorf("expected first script install-go.sh, got %v", executed[0].args)
	}
	if executed[1].args[1] != "scripts/install-linters.sh" {
		t.Errorf("expected second script install-linters.sh, got %v", executed[1].args)
	}
}

func TestLimaBackendCreatePassesEnvVarsToVMIsolationScripts(t *testing.T) {
	fix := newVMTestFixture(t)
	fix.writePidYaml([]byte(`isolarium:
  vm:
    create:
      creation_scripts:
        - path: scripts/install-codescene.sh
          env:
            - CS_ACCESS_TOKEN
`))

	t.Setenv("CS_ACCESS_TOKEN", "my-token")

	var executed []recordedVMExec
	b := &LimaBackend{
		CreateVMFunc:  noopCreateVM,
		VMHomeDirFunc: stubVMHomeDir,
		VMExecFunc: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
			executed = append(executed, recordedVMExec{vm: vm, workdir: workdir, envVars: envVars, args: args})
			return 0, nil
		},
	}

	err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executed[0].envVars["CS_ACCESS_TOKEN"] != "my-token" {
		t.Errorf("expected CS_ACCESS_TOKEN=my-token, got %q", executed[0].envVars["CS_ACCESS_TOKEN"])
	}
}

func TestLimaBackendCreateIsolationScriptErrors(t *testing.T) {
	tests := []struct {
		name        string
		pidYaml     string
		executor    lima.VMExecFunc
		expectedErr string
	}{
		{
			name: "missing env vars prevents execution",
			pidYaml: `isolarium:
  vm:
    create:
      creation_scripts:
        - path: scripts/install-codescene.sh
          env:
            - NONEXISTENT_VAR`,
			executor: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
				t.Fatal("executor should not be called when env vars are missing")
				return 0, nil
			},
			expectedErr: "NONEXISTENT_VAR",
		},
		{
			name: "script failure propagates as create error",
			pidYaml: `isolarium:
  vm:
    create:
      creation_scripts:
        - path: scripts/install-go.sh`,
			executor: func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
				return 1, fmt.Errorf("script failed")
			},
			expectedErr: "install-go.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fix := newVMTestFixture(t)
			fix.writePidYaml([]byte(tt.pidYaml))

			b := &LimaBackend{
				CreateVMFunc:  noopCreateVM,
				VMHomeDirFunc: stubVMHomeDir,
				VMExecFunc:    tt.executor,
			}

			err := b.Create(CreateOptions{Name: "test-vm", WorkDirectory: fix.workDir})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error to contain %q, got: %s", tt.expectedErr, err.Error())
			}
		})
	}
}
