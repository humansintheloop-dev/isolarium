//go:build integration

package lima

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/github"
)

// Aliases for github package functions used in tests
var parseRepoURL = github.ParseRepoURL
var newTokenMinter = github.NewTokenMinter

// loadTestEnvFile loads .env.local from the project root
func loadTestEnvFile(t *testing.T) {
	t.Helper()
	envPath := findProjectRoot(t) + "/.env.local"
	file, err := os.Open(envPath)
	if err != nil {
		t.Logf(".env.local not found at %s, skipping", envPath)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}

// Integration tests for Lima VM management
// These tests require Lima to be installed and can take several minutes to run
// Run with: go test -tags=integration ./internal/lima/...

func TestCreateAndDestroyVM_Integration(t *testing.T) {
	// Skip if Lima is not installed
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("Lima not installed, skipping integration test")
	}

	// Clean up any existing VM first
	_ = DestroyVM()

	// Create VM
	err := CreateVM()
	if err != nil {
		t.Fatalf("CreateVM failed: %v", err)
	}

	// Verify VM exists and is running
	cmd := exec.Command("limactl", "list", "--format", "{{.Name}}:{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to list VMs: %v", err)
	}
	if !strings.Contains(string(output), "isolarium:Running") {
		t.Errorf("expected VM to be running, got: %s", output)
	}

	// Clean up
	err = DestroyVM()
	if err != nil {
		t.Fatalf("DestroyVM failed: %v", err)
	}

	// Verify VM is gone
	output, _ = exec.Command("limactl", "list", "--format", "{{.Name}}").Output()
	if strings.Contains(string(output), "isolarium") {
		t.Errorf("VM still exists after destroy: %s", output)
	}
}

func TestVMHasRequiredTools_Integration(t *testing.T) {
	// Skip if Lima is not installed
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("Lima not installed, skipping integration test")
	}

	// This test assumes VM is already created (run after TestCreateAndDestroyVM_Integration)
	// Check for required tools (all should be in PATH via symlinks or direct install)
	tools := []string{"git", "node", "docker", "gh", "claude", "java"}
	for _, tool := range tools {
		cmd := exec.Command("limactl", "shell", "isolarium", "--", "which", tool)
		if err := cmd.Run(); err != nil {
			t.Errorf("tool %s not found in VM", tool)
		}
	}

	// Check JAVA_HOME is set in /etc/environment
	cmd := exec.Command("limactl", "shell", "isolarium", "--", "grep", "JAVA_HOME", "/etc/environment")
	if err := cmd.Run(); err != nil {
		t.Error("JAVA_HOME not set in /etc/environment")
	}
}

func TestCloneRepoWithToken_Integration(t *testing.T) {
	// Skip if Lima is not installed
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("Lima not installed, skipping integration test")
	}

	// Ensure VM is running, create if necessary
	ensureVMRunning(t)

	// Get the expected branch from the host repo
	branchCmd := exec.Command("git", "branch", "--show-current")
	branchOutput, err := branchCmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	expectedBranch := strings.TrimSpace(string(branchOutput))

	// Get the expected remote URL
	remoteCmd := exec.Command("git", "remote", "get-url", "origin")
	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		t.Fatalf("failed to get remote URL: %v", err)
	}
	remoteURL := strings.TrimSpace(string(remoteOutput))
	t.Logf("Testing with repo: %s, branch: %s", remoteURL, expectedBranch)

	// Remove any existing repo directory
	exec.Command("limactl", "shell", "isolarium", "--", "rm", "-rf", "repo").Run()

	// Load .env.local to get GitHub App credentials
	loadTestEnvFile(t)

	// Mint token if credentials are available
	var token string
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if appID != "" && privateKeyPath != "" {
		privateKeyBytes, err := os.ReadFile(privateKeyPath)
		if err != nil {
			t.Fatalf("failed to read private key: %v", err)
		}

		// Parse owner/repo from URL
		owner, repo, err := parseRepoURL(remoteURL)
		if err != nil {
			t.Fatalf("failed to parse repo URL: %v", err)
		}

		minter, err := newTokenMinter(appID, string(privateKeyBytes), "")
		if err != nil {
			t.Fatalf("failed to create token minter: %v", err)
		}

		token, err = minter.MintInstallationToken(owner, repo)
		if err != nil {
			t.Fatalf("failed to mint token: %v", err)
		}
		t.Log("Token minted successfully")
	} else {
		t.Fatal("GitHub App credentials not configured - set GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY_PATH in .env.local")
	}

	// Clone the repository
	if err := CloneRepo(remoteURL, expectedBranch, token); err != nil {
		t.Fatalf("CloneRepo failed: %v", err)
	}

	// Verify repo was cloned by checking for go.mod
	cmd := exec.Command("limactl", "shell", "isolarium", "--", "test", "-f", "repo/go.mod")
	if err := cmd.Run(); err != nil {
		t.Error("go.mod not found in cloned repo - repo was not cloned")
	}

	// Verify correct branch is checked out
	cmd = exec.Command("limactl", "shell", "isolarium", "--", "git", "-C", "repo", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch in VM: %v", err)
	}
	actualBranch := strings.TrimSpace(string(output))
	if actualBranch != expectedBranch {
		t.Errorf("expected branch %q, got %q", expectedBranch, actualBranch)
	}

	// Write and verify metadata
	owner, repo, _ := parseRepoURL(remoteURL)
	if err := WriteRepoMetadata(owner, repo, expectedBranch); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}

	cmd = exec.Command("limactl", "shell", "isolarium", "--", "cat", ".isolarium/repo.json")
	metadataOutput, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to read metadata: %v", err)
	}

	var meta RepoMetadata
	if err := json.Unmarshal(metadataOutput, &meta); err != nil {
		t.Fatalf("failed to parse metadata: %v", err)
	}

	if meta.Branch != expectedBranch {
		t.Errorf("metadata branch: expected %q, got %q", expectedBranch, meta.Branch)
	}
	if meta.Owner == "" || meta.Repo == "" {
		t.Errorf("metadata missing owner/repo: %+v", meta)
	}
	t.Logf("Metadata: owner=%s, repo=%s, branch=%s", meta.Owner, meta.Repo, meta.Branch)
}

// findProjectRoot returns the project root directory
func findProjectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func TestWriteAndReadMetadata_Integration(t *testing.T) {
	// Skip if Lima is not installed
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("Lima not installed, skipping integration test")
	}

	// Ensure VM is running, create if necessary
	ensureVMRunning(t)

	// Write metadata
	if err := WriteRepoMetadata("testowner", "testrepo", "main"); err != nil {
		t.Fatalf("WriteRepoMetadata failed: %v", err)
	}

	// Read metadata back
	meta, err := ReadRepoMetadata()
	if err != nil {
		t.Fatalf("ReadRepoMetadata failed: %v", err)
	}

	if meta.Owner != "testowner" {
		t.Errorf("expected owner 'testowner', got %q", meta.Owner)
	}
	if meta.Repo != "testrepo" {
		t.Errorf("expected repo 'testrepo', got %q", meta.Repo)
	}
	if meta.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", meta.Branch)
	}

	// Verify file exists in VM
	cmd := exec.Command("limactl", "shell", "isolarium", "--", "cat", ".isolarium/repo.json")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to read metadata file from VM: %v", err)
	}

	var fileMeta RepoMetadata
	if err := json.Unmarshal(output, &fileMeta); err != nil {
		t.Fatalf("failed to parse metadata from VM: %v", err)
	}
	if fileMeta.Owner != "testowner" {
		t.Errorf("file metadata owner mismatch: got %q", fileMeta.Owner)
	}
}

// ensureVMRunning checks if the VM exists and is running, creating it if necessary
func ensureVMRunning(t *testing.T) {
	t.Helper()

	exists, err := VMExists()
	if err != nil {
		t.Fatalf("failed to check VM status: %v", err)
	}

	if !exists {
		t.Log("VM does not exist, creating...")
		if err := CreateVM(); err != nil {
			t.Fatalf("failed to create VM: %v", err)
		}
	}

	// Verify VM is running
	cmd := exec.Command("limactl", "list", "--format", "{{.Name}}:{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to check VM status: %v", err)
	}
	if !strings.Contains(string(output), "isolarium:Running") {
		t.Fatalf("VM is not running: %s", output)
	}
}
