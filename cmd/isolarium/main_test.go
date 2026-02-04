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

func TestLoadEnvFile_LoadsVariables(t *testing.T) {
	// Create a temp directory and env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	content := `GITHUB_APP_ID=12345
GITHUB_APP_PRIVATE_KEY_PATH=/path/to/key.pem
# This is a comment
ANOTHER_VAR=value with spaces
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Clear any existing values
	os.Unsetenv("GITHUB_APP_ID")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	os.Unsetenv("ANOTHER_VAR")

	// Load the env file
	loadEnvFile(envFile)

	// Verify variables were loaded
	if got := os.Getenv("GITHUB_APP_ID"); got != "12345" {
		t.Errorf("GITHUB_APP_ID: expected '12345', got '%s'", got)
	}
	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != "/path/to/key.pem" {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '/path/to/key.pem', got '%s'", got)
	}
	if got := os.Getenv("ANOTHER_VAR"); got != "value with spaces" {
		t.Errorf("ANOTHER_VAR: expected 'value with spaces', got '%s'", got)
	}

	// Clean up
	os.Unsetenv("GITHUB_APP_ID")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	os.Unsetenv("ANOTHER_VAR")
}

func TestLoadEnvFile_DoesNotOverrideExisting(t *testing.T) {
	// Create a temp env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	content := `GITHUB_APP_ID=from-file`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Set existing value
	os.Setenv("GITHUB_APP_ID", "from-environment")
	defer os.Unsetenv("GITHUB_APP_ID")

	// Load the env file
	loadEnvFile(envFile)

	// Existing value should NOT be overridden
	if got := os.Getenv("GITHUB_APP_ID"); got != "from-environment" {
		t.Errorf("GITHUB_APP_ID should not be overridden: expected 'from-environment', got '%s'", got)
	}
}

func TestLoadEnvFile_HandlesNonexistentFile(t *testing.T) {
	// Should not panic or error on missing file
	loadEnvFile("/nonexistent/path/.env.local")
	// If we get here, the test passes
}

func TestRunCommand_HasCopySessionFlag(t *testing.T) {
	// Build the binary first
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	// Run 'run --help' to verify the flag exists
	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "--copy-session") {
		t.Errorf("expected 'run' command to have '--copy-session' flag, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "--script") {
		t.Errorf("expected 'run' command to have '--script' flag, got: %s", outputStr)
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
