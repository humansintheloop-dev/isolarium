package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrNotGitRepository = errors.New("not a git repository")

// GetRemoteURL returns the URL of the origin remote for the git repository at the given path
func GetRemoteURL(path string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", ErrNotGitRepository
	}
	return strings.TrimSpace(string(output)), nil
}

func PushBranch(path, branch string) error {
	cmd := exec.Command("git", "push", "-u", "origin", branch)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch %s: %w\n%s", branch, err, output)
	}
	return nil
}

// GetCurrentBranch returns the current branch name for the git repository at the given path
func GetCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", ErrNotGitRepository
	}
	return strings.TrimSpace(string(output)), nil
}
