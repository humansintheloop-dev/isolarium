package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetRemoteURL(t *testing.T) {
	// Create a temporary git repository
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

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
	tmpDir := createNonGitDir(t)
	_, err := GetRemoteURL(tmpDir)
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
	defer func() { _ = os.RemoveAll(tmpDir) }()

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
	tmpDir := createNonGitDir(t)
	_, err := GetCurrentBranch(tmpDir)
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

func TestPushBranch(t *testing.T) {
	bareDir := createBareRemote(t)
	workDir := cloneFromBare(t, bareDir)

	if err := runGitCommand(workDir, "checkout", "-b", "feature-x"); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	testFile := filepath.Join(workDir, "new.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := runGitCommand(workDir, "add", "new.txt"); err != nil {
		t.Fatalf("failed to add: %v", err)
	}
	if err := runGitCommand(workDir, "commit", "-m", "new commit"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	if err := PushBranch(workDir, "feature-x"); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}

	verifyBranchExistsOnRemote(t, bareDir, "feature-x")
}

func TestPushBranch_NotGitRepo(t *testing.T) {
	tmpDir := createNonGitDir(t)
	if err := PushBranch(tmpDir, "main"); err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

func createBareRemote(t *testing.T) string {
	t.Helper()
	bareDir, err := os.MkdirTemp("", "bare-remote-*")
	if err != nil {
		t.Fatalf("failed to create bare dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(bareDir) })
	if err := runGitCommand(bareDir, "init", "--bare"); err != nil {
		t.Fatalf("failed to init bare repo: %v", err)
	}
	return bareDir
}

func cloneFromBare(t *testing.T, bareDir string) string {
	t.Helper()
	workDir, err := os.MkdirTemp("", "work-clone-*")
	if err != nil {
		t.Fatalf("failed to create work dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	cmd := exec.Command("git", "clone", bareDir, workDir+"/repo")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone: %v\n%s", err, out)
	}
	repoDir := workDir + "/repo"

	configureTestGitUser(t, repoDir)
	commitAndPushInitial(t, repoDir)
	return repoDir
}

func configureTestGitUser(t *testing.T, repoDir string) {
	t.Helper()
	if err := runGitCommand(repoDir, "config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("failed to configure email: %v", err)
	}
	if err := runGitCommand(repoDir, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("failed to configure name: %v", err)
	}
}

func commitAndPushInitial(t *testing.T, repoDir string) {
	t.Helper()
	initFile := filepath.Join(repoDir, "init.txt")
	if err := os.WriteFile(initFile, []byte("init"), 0644); err != nil {
		t.Fatalf("failed to write init file: %v", err)
	}
	if err := runGitCommand(repoDir, "add", "."); err != nil {
		t.Fatalf("failed to add: %v", err)
	}
	if err := runGitCommand(repoDir, "commit", "-m", "initial"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	if err := runGitCommand(repoDir, "push", "-u", "origin", "main"); err != nil {
		_ = runGitCommand(repoDir, "push", "-u", "origin", "master")
	}
}

func verifyBranchExistsOnRemote(t *testing.T, bareDir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", branch)
	cmd.Dir = bareDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to list branches: %v", err)
	}
	if !strings.Contains(string(out), branch) {
		t.Errorf("branch %q not found on remote", branch)
	}
}

func createNonGitDir(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "not-git-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	return tmpDir
}

func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
