//go:build e2e_pytest

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPytestInVM_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	projectRoot := pytestProjectRoot(t)
	pythonCliDir := filepath.Join(projectRoot, "testdata", "python-cli-app")

	tmpDir := copyToTempGitRepo(t, pythonCliDir)

	createVMFromTempRepo(t, binary, tmpDir, projectRoot)
	t.Cleanup(func() { destroyVM(t, binary, tmpDir) })

	pytestCmd := "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run pytest -v"
	pytestArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", pytestCmd}
	cmd := exec.Command(binary, pytestArgs...)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	t.Logf("pytest output:\n%s", output)
	if err != nil {
		t.Fatalf("pytest in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "2 passed") {
		t.Fatal("expected 2 passed in output")
	}
}

func TestGreeterCliInVM_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	projectRoot := pytestProjectRoot(t)
	pythonCliDir := filepath.Join(projectRoot, "testdata", "python-cli-app")

	tmpDir := copyToTempGitRepo(t, pythonCliDir)

	createVMFromTempRepo(t, binary, tmpDir, projectRoot)
	t.Cleanup(func() { destroyVM(t, binary, tmpDir) })

	greeterCmd := "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run greeter VM"
	greeterArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", greeterCmd}
	cmd := exec.Command(binary, greeterArgs...)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	t.Logf("greeter output:\n%s", output)
	if err != nil {
		t.Fatalf("greeter CLI in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "Hello, VM!") {
		t.Fatal("expected 'Hello, VM!' in output")
	}
}
