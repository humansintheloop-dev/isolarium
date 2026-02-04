package github

import (
	"errors"
	"strings"
)

var ErrInvalidGitHubURL = errors.New("invalid GitHub URL")

// ParseRepoURL extracts owner and repo from a GitHub URL.
// Supports both HTTPS (https://github.com/owner/repo.git) and
// SSH (git@github.com:owner/repo.git) formats.
func ParseRepoURL(remoteURL string) (owner, repo string, err error) {
	// Remove .git suffix if present
	url := strings.TrimSuffix(remoteURL, ".git")

	// Handle SSH format: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@github.com:") {
		path := strings.TrimPrefix(url, "git@github.com:")
		return parseOwnerRepo(path)
	}

	// Handle HTTPS format: https://github.com/owner/repo
	if strings.HasPrefix(url, "https://github.com/") {
		path := strings.TrimPrefix(url, "https://github.com/")
		return parseOwnerRepo(path)
	}

	return "", "", ErrInvalidGitHubURL
}

func parseOwnerRepo(path string) (owner, repo string, err error) {
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrInvalidGitHubURL
	}
	return parts[0], parts[1], nil
}
