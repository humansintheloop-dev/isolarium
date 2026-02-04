package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStatusCommand_OutputsVMNone(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	// Run the status command
	cmd := exec.Command("./isolarium", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status command failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "VM: none") {
		t.Errorf("expected output to contain 'VM: none', got: %s", outputStr)
	}
}

func TestStatusCommand_OutputsGitHubAppNotConfigured(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	// Run the status command
	cmd := exec.Command("./isolarium", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status command failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "GitHub App: not configured") {
		t.Errorf("expected output to contain 'GitHub App: not configured', got: %s", outputStr)
	}
}
