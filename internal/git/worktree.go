package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WorktreeInfo struct {
	MainRepoDir string
	WorktreeDir string
}

func DetectWorktree(workDir string) (*WorktreeInfo, error) {
	gitPath := filepath.Join(workDir, ".git")

	fi, err := os.Lstat(gitPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		return nil, nil
	}

	content, err := os.ReadFile(gitPath)
	if err != nil {
		return nil, err
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return nil, fmt.Errorf("malformed .git file: expected 'gitdir: ' prefix, got %q", line)
	}

	gitdir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(workDir, gitdir)
	}

	gitdir, err = filepath.Abs(gitdir)
	if err != nil {
		return nil, err
	}

	gitdir, err = filepath.EvalSymlinks(gitdir)
	if err != nil {
		return nil, err
	}

	// Worktree gitdir is typically <main-repo>/.git/worktrees/<name>
	// Walk up: parent of gitdir is "worktrees", parent of that is ".git", parent of that is the main repo
	mainGitDir := filepath.Dir(filepath.Dir(gitdir))
	mainRepoDir := filepath.Dir(mainGitDir)

	absWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return nil, err
	}

	return &WorktreeInfo{
		MainRepoDir: mainRepoDir,
		WorktreeDir: absWorkDir,
	}, nil
}
