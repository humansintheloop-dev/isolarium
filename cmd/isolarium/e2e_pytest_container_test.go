//go:build e2e_pytest

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const pytestTestContainerName = "isolarium-pytest-test"

func TestPytestInContainer_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	projectRoot := pytestProjectRoot(t)
	pythonCliDir := filepath.Join(projectRoot, "testdata", "python-cli-app")

	createContainerForPytest(t, binary, pythonCliDir)
	t.Cleanup(func() { destroyContainerForPytest(t, binary, pythonCliDir) })

	pytestCmd := "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run pytest -v"
	pytestArgs := []string{"--type", "container", "--name", pytestTestContainerName, "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", pytestCmd}
	cmd := exec.Command(binary, pytestArgs...)
	cmd.Dir = pythonCliDir
	output, err := cmd.CombinedOutput()
	t.Logf("pytest output:\n%s", output)
	if err != nil {
		t.Fatalf("pytest in container failed: %v", err)
	}
	if !strings.Contains(string(output), "2 passed") {
		t.Fatal("expected 2 passed in output")
	}
}

func TestGreeterCliInContainer_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	projectRoot := pytestProjectRoot(t)
	pythonCliDir := filepath.Join(projectRoot, "testdata", "python-cli-app")

	createContainerForPytest(t, binary, pythonCliDir)
	t.Cleanup(func() { destroyContainerForPytest(t, binary, pythonCliDir) })

	greeterCmd := "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run greeter Container"
	greeterArgs := []string{"--type", "container", "--name", pytestTestContainerName, "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", greeterCmd}
	cmd := exec.Command(binary, greeterArgs...)
	cmd.Dir = pythonCliDir
	output, err := cmd.CombinedOutput()
	t.Logf("greeter output:\n%s", output)
	if err != nil {
		t.Fatalf("greeter CLI in container failed: %v", err)
	}
	if !strings.Contains(string(output), "Hello, Container!") {
		t.Fatal("expected 'Hello, Container!' in output")
	}
}

func createContainerForPytest(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "container", "--name", pytestTestContainerName, "create")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("container create output:\n%s", output)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
}

func destroyContainerForPytest(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "container", "--name", pytestTestContainerName, "destroy")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("container destroy output:\n%s", output)
	if err != nil {
		t.Logf("container destroy failed (ignoring): %v", err)
	}
}
