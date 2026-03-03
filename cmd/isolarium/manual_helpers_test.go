//go:build manual

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func buildBinary(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	binaryPath := filepath.Join(root, "bin", "isolarium")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/isolarium")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}
	return binaryPath
}

func ensureEnvironmentReady(t *testing.T, binary, envType string) {
	t.Helper()
	if envType == "nono" {
		return
	}
	cmd := exec.Command(binary, "--type", envType, "status")
	output, _ := cmd.Output()
	if strings.Contains(string(output), "running") {
		return
	}
	t.Logf("creating %s environment...", envType)
	cmd = exec.Command(binary, "--type", envType, "create")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to create %s environment: %v\n%s", envType, err, out)
	}
}

func claudeInIsolarium(t *testing.T, envType string) string {
	t.Helper()
	binary := buildBinary(t)
	ensureEnvironmentReady(t, binary, envType)
	cmd := exec.Command(binary, "--type", envType, "run", "--", "claude", "-p", "hello")
	cmd.Dir = projectRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolarium --type %s run -- claude -p hello failed: %v\noutput: %s", envType, err, output)
	}
	return string(output)
}

func verifyClaudeResponded(t *testing.T, output string) {
	t.Helper()
	t.Logf("Claude response:\n%s", output)
	trimmed := strings.TrimSpace(output)
	if len(trimmed) == 0 {
		t.Fatal("expected non-empty response from claude")
	}
}
