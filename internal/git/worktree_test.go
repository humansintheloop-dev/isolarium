package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectWorktreeReturnsNilForNormalRepo(t *testing.T) {
	repoDir := createTempGitRepo(t)

	info, err := DetectWorktree(repoDir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for normal repo, got %+v", info)
	}
}

func TestDetectWorktreeReturnsInfoForWorktree(t *testing.T) {
	repoDir := createTempGitRepoWithCommit(t)

	worktreeDir := filepath.Join(evalSymlinksOrFail(t, t.TempDir()), "my-worktree")
	runGitCommandOrFail(t, repoDir, "worktree", "add", worktreeDir, "-b", "worktree-branch")

	info, err := DetectWorktree(worktreeDir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected WorktreeInfo for worktree, got nil")
	}
	if info.MainRepoDir != repoDir {
		t.Errorf("expected MainRepoDir %q, got %q", repoDir, info.MainRepoDir)
	}
	if info.WorktreeDir != worktreeDir {
		t.Errorf("expected WorktreeDir %q, got %q", worktreeDir, info.WorktreeDir)
	}
}

func TestDetectWorktreeReturnsNilForNonGitDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	info, err := DetectWorktree(tmpDir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for non-git directory, got %+v", info)
	}
}

func TestDetectWorktreeReturnsErrorForMalformedGitFile(t *testing.T) {
	tmpDir := t.TempDir()
	gitFilePath := filepath.Join(tmpDir, ".git")
	if err := os.WriteFile(gitFilePath, []byte("this is not a valid gitdir pointer"), 0644); err != nil {
		t.Fatalf("failed to write malformed .git file: %v", err)
	}

	_, err := DetectWorktree(tmpDir)

	if err == nil {
		t.Error("expected error for malformed .git file, got nil")
	}
}

func createTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := evalSymlinksOrFail(t, t.TempDir())
	runGitCommandOrFail(t, dir, "init")
	return dir
}

func createTempGitRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := createTempGitRepo(t)
	runGitCommandOrFail(t, dir, "config", "user.email", "test@test.com")
	runGitCommandOrFail(t, dir, "config", "user.name", "Test User")
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	runGitCommandOrFail(t, dir, "add", "test.txt")
	runGitCommandOrFail(t, dir, "commit", "-m", "initial commit")
	return dir
}

func runGitCommandOrFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func evalSymlinksOrFail(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for %q: %v", path, err)
	}
	return resolved
}
