//go:build e2e_pytest

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPytestInNono_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	pythonCliDir := filepath.Join(pytestProjectRoot(t), "testdata", "python-cli-app")

	cmd := exec.Command(binary, "--type", "nono", "run", "--no-gh-token", "--", "uv", "run", "pytest", "-v")
	cmd.Dir = pythonCliDir
	output, err := cmd.CombinedOutput()
	t.Logf("pytest output:\n%s", output)
	if err != nil {
		t.Fatalf("pytest in nono failed: %v", err)
	}
	if !strings.Contains(string(output), "2 passed") {
		t.Fatal("expected 2 passed in output")
	}
}

func TestGreeterCliInNono_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	pythonCliDir := filepath.Join(pytestProjectRoot(t), "testdata", "python-cli-app")

	cmd := exec.Command(binary, "--type", "nono", "run", "--no-gh-token", "--", "uv", "run", "greeter", "Nono")
	cmd.Dir = pythonCliDir
	output, err := cmd.CombinedOutput()
	t.Logf("greeter output:\n%s", output)
	if err != nil {
		t.Fatalf("greeter CLI in nono failed: %v", err)
	}
	if !strings.Contains(string(output), "Hello, Nono!") {
		t.Fatal("expected 'Hello, Nono!' in output")
	}
}

func pytestProjectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func buildPytestBinary(t *testing.T) string {
	t.Helper()
	root := pytestProjectRoot(t)
	binaryPath := filepath.Join(root, "bin", "isolarium")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/isolarium")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}
	return binaryPath
}
