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
	pythonCliDir := filepath.Join(pytestProjectRoot(t), "testdata", "python-cli-app")

	createVMForPytest(t, binary, pythonCliDir)
	t.Cleanup(func() { destroyVMForPytest(t, binary, pythonCliDir) })

	pytestCmd := "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run pytest -v"
	pytestArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", pytestCmd}
	cmd := exec.Command(binary, pytestArgs...)
	cmd.Dir = pythonCliDir
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
	pythonCliDir := filepath.Join(pytestProjectRoot(t), "testdata", "python-cli-app")

	createVMForPytest(t, binary, pythonCliDir)
	t.Cleanup(func() { destroyVMForPytest(t, binary, pythonCliDir) })

	greeterCmd := "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run greeter VM"
	greeterArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", greeterCmd}
	cmd := exec.Command(binary, greeterArgs...)
	cmd.Dir = pythonCliDir
	output, err := cmd.CombinedOutput()
	t.Logf("greeter output:\n%s", output)
	if err != nil {
		t.Fatalf("greeter CLI in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "Hello, VM!") {
		t.Fatal("expected 'Hello, VM!' in output")
	}
}

func createVMForPytest(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "vm", "create")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM create output:\n%s", output)
	if err != nil {
		t.Fatalf("VM create failed: %v", err)
	}
}

func destroyVMForPytest(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "vm", "destroy")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM destroy output:\n%s", output)
	if err != nil {
		t.Logf("VM destroy failed (ignoring): %v", err)
	}
}
