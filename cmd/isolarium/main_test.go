package main

import (
	"fmt"
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

func TestLoadEnvFile_LoadsVariables(t *testing.T) {
	// Create a temp directory and env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	// Create a real file for the _PATH variable
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(keyFile, []byte("key content"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	content := fmt.Sprintf(`GITHUB_APP_ID=12345
GITHUB_APP_PRIVATE_KEY_PATH=%s
# This is a comment
ANOTHER_VAR=value with spaces
`, keyFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Clear any existing values
	os.Unsetenv("GITHUB_APP_ID")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	os.Unsetenv("ANOTHER_VAR")

	// Load the env file
	if err := loadEnvFile(envFile); err != nil {
		t.Fatalf("loadEnvFile failed: %v", err)
	}

	// Verify variables were loaded
	if got := os.Getenv("GITHUB_APP_ID"); got != "12345" {
		t.Errorf("GITHUB_APP_ID: expected '12345', got '%s'", got)
	}
	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != keyFile {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '%s', got '%s'", keyFile, got)
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
	if err := loadEnvFile(envFile); err != nil {
		t.Fatalf("loadEnvFile failed: %v", err)
	}

	// Existing value should NOT be overridden
	if got := os.Getenv("GITHUB_APP_ID"); got != "from-environment" {
		t.Errorf("GITHUB_APP_ID should not be overridden: expected 'from-environment', got '%s'", got)
	}
}

func TestLoadEnvFile_HandlesNonexistentFile(t *testing.T) {
	// Should not panic or error on missing env file
	err := loadEnvFile("/nonexistent/path/.env.local")
	if err != nil {
		t.Errorf("expected no error for missing env file, got: %v", err)
	}
}

func TestLoadEnvFile_ValidatesPathVariablesExist(t *testing.T) {
	// Create a temp directory and env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	// Create a real file that the PATH variable will reference
	realFile := filepath.Join(tmpDir, "real-key.pem")
	if err := os.WriteFile(realFile, []byte("key content"), 0600); err != nil {
		t.Fatalf("failed to write real file: %v", err)
	}

	content := fmt.Sprintf(`GITHUB_APP_PRIVATE_KEY_PATH=%s
`, realFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Clear any existing values
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	// Load the env file - should succeed because file exists
	err := loadEnvFile(envFile)
	if err != nil {
		t.Errorf("expected no error when PATH file exists, got: %v", err)
	}

	// Verify variable was loaded
	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != realFile {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '%s', got '%s'", realFile, got)
	}

	// Clean up
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
}

func TestLoadEnvFile_ErrorsWhenPathVariableFileNotFound(t *testing.T) {
	// Create a temp directory and env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	// Reference a non-existent file
	nonExistentFile := filepath.Join(tmpDir, "non-existent-key.pem")

	content := fmt.Sprintf(`GITHUB_APP_PRIVATE_KEY_PATH=%s
`, nonExistentFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Clear any existing values
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	// Load the env file - should fail because referenced file doesn't exist
	err := loadEnvFile(envFile)
	if err == nil {
		t.Error("expected error when PATH file doesn't exist, got nil")
	}

	// Error message should mention the variable name and the path
	if err != nil {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "GITHUB_APP_PRIVATE_KEY_PATH") {
			t.Errorf("error should mention variable name, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, nonExistentFile) {
			t.Errorf("error should mention file path, got: %s", errMsg)
		}
	}

	// Clean up
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
}

func TestLoadEnvFile_ValidatesMultiplePathVariables(t *testing.T) {
	// Create a temp directory and env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	// Create real files
	keyFile := filepath.Join(tmpDir, "key.pem")
	certFile := filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(keyFile, []byte("key"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	if err := os.WriteFile(certFile, []byte("cert"), 0600); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}

	content := fmt.Sprintf(`GITHUB_APP_PRIVATE_KEY_PATH=%s
SOME_CERT_PATH=%s
REGULAR_VAR=not_a_path
`, keyFile, certFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Clear any existing values
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	os.Unsetenv("SOME_CERT_PATH")
	os.Unsetenv("REGULAR_VAR")

	// Load the env file - should succeed
	err := loadEnvFile(envFile)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify all variables were loaded
	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != keyFile {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '%s', got '%s'", keyFile, got)
	}
	if got := os.Getenv("SOME_CERT_PATH"); got != certFile {
		t.Errorf("SOME_CERT_PATH: expected '%s', got '%s'", certFile, got)
	}
	if got := os.Getenv("REGULAR_VAR"); got != "not_a_path" {
		t.Errorf("REGULAR_VAR: expected 'not_a_path', got '%s'", got)
	}

	// Clean up
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	os.Unsetenv("SOME_CERT_PATH")
	os.Unsetenv("REGULAR_VAR")
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
}

func TestRunCommand_UsageShowsCommandSyntax(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	// The usage should show that run accepts args after --
	if !strings.Contains(outputStr, "[flags] -- command") {
		t.Errorf("expected usage to show '-- command' syntax, got: %s", outputStr)
	}
}

func TestRunCommand_FailsWithNoCommand(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	// Run 'run' with no args after -- should fail with a helpful message
	cmd := exec.Command(binaryPath, "run", "--copy-session=false")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected run with no command to fail, got: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "no command specified") {
		t.Errorf("expected error about no command, got: %s", outputStr)
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
