package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestDestroyCommand_SucceedsWhenVMExists(t *testing.T) {
	// Get the current working directory to build absolute path
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	binaryPath := filepath.Join(cwd, "isolarium")

	// Run destroy command (VM exists from previous tests)
	cmd := exec.Command(binaryPath, "destroy")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("destroy command failed: %v, output: %s", err, output)
	}

	// Verify VM is gone
	listCmd := exec.Command("limactl", "list", "--format", "{{.Name}}")
	listOutput, _ := listCmd.Output()
	if strings.Contains(string(listOutput), "isolarium") {
		t.Error("VM still exists after destroy")
	}
}

func TestCreateCommand_FailsWhenNotInGitRepo(t *testing.T) {
	// Get the current working directory to build absolute path
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	// Get absolute path to the binary
	binaryPath := filepath.Join(cwd, "isolarium")

	// Create a temporary directory that is NOT a git repo
	tmpDir := t.TempDir()

	// Run the create command from the non-git directory using absolute path
	cmd := exec.Command(binaryPath, "create")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()

	// Command should fail
	if err == nil {
		t.Fatalf("expected create command to fail in non-git directory, but it succeeded. Output: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "not a git repository") {
		t.Errorf("expected error message to contain 'not a git repository', got: %s", outputStr)
	}
}
