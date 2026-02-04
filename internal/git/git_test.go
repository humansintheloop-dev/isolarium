package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetRemoteURL(t *testing.T) {
	// Create a temporary git repository
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	if err := runGitCommand(tmpDir, "init"); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Add a remote
	expectedURL := "git@github.com:owner/repo.git"
	if err := runGitCommand(tmpDir, "remote", "add", "origin", expectedURL); err != nil {
		t.Fatalf("failed to add remote: %v", err)
	}

	// Test GetRemoteURL
	url, err := GetRemoteURL(tmpDir)
	if err != nil {
		t.Fatalf("GetRemoteURL failed: %v", err)
	}
	if url != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, url)
	}
}

func TestGetRemoteURL_NotGitRepo(t *testing.T) {
	// Create a temporary directory that is NOT a git repository
	tmpDir, err := os.MkdirTemp("", "not-git-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test GetRemoteURL on non-git directory
	_, err = GetRemoteURL(tmpDir)
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	// Create a temporary git repository
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	if err := runGitCommand(tmpDir, "init"); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for commit
	if err := runGitCommand(tmpDir, "config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("failed to configure git email: %v", err)
	}
	if err := runGitCommand(tmpDir, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("failed to configure git name: %v", err)
	}

	// Create initial commit to establish a branch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := runGitCommand(tmpDir, "add", "test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}
	if err := runGitCommand(tmpDir, "commit", "-m", "initial commit"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create and checkout a feature branch
	expectedBranch := "feature-test"
	if err := runGitCommand(tmpDir, "checkout", "-b", expectedBranch); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	// Test GetCurrentBranch
	branch, err := GetCurrentBranch(tmpDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != expectedBranch {
		t.Errorf("expected branch %q, got %q", expectedBranch, branch)
	}
}

func TestGetCurrentBranch_NotGitRepo(t *testing.T) {
	// Create a temporary directory that is NOT a git repository
	tmpDir, err := os.MkdirTemp("", "not-git-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test GetCurrentBranch on non-git directory
	_, err = GetCurrentBranch(tmpDir)
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
