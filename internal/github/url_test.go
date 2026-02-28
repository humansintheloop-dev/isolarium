package github

import (
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/project"
)

func TestParseRepoURL_HTTPS(t *testing.T) {
	owner, repo, err := ParseRepoURL("https://github.com/" + project.GitHubOrgRepo + ".git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != project.GitHubOrg {
		t.Errorf("expected owner %q, got %q", project.GitHubOrg, owner)
	}
	if repo != project.GitHubRepo {
		t.Errorf("expected repo %q, got %q", project.GitHubRepo, repo)
	}
}

func TestParseRepoURL_HTTPSWithoutGitSuffix(t *testing.T) {
	owner, repo, err := ParseRepoURL("https://github.com/" + project.GitHubOrgRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != project.GitHubOrg {
		t.Errorf("expected owner %q, got %q", project.GitHubOrg, owner)
	}
	if repo != project.GitHubRepo {
		t.Errorf("expected repo %q, got %q", project.GitHubRepo, repo)
	}
}

func TestParseRepoURL_SSH(t *testing.T) {
	owner, repo, err := ParseRepoURL("git@github.com:" + project.GitHubOrgRepo + ".git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != project.GitHubOrg {
		t.Errorf("expected owner %q, got %q", project.GitHubOrg, owner)
	}
	if repo != project.GitHubRepo {
		t.Errorf("expected repo %q, got %q", project.GitHubRepo, repo)
	}
}

func TestParseRepoURL_SSHWithoutGitSuffix(t *testing.T) {
	owner, repo, err := ParseRepoURL("git@github.com:" + project.GitHubOrgRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != project.GitHubOrg {
		t.Errorf("expected owner %q, got %q", project.GitHubOrg, owner)
	}
	if repo != project.GitHubRepo {
		t.Errorf("expected repo %q, got %q", project.GitHubRepo, repo)
	}
}

func TestParseRepoURL_InvalidURL(t *testing.T) {
	_, _, err := ParseRepoURL("not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestParseRepoURL_NonGitHubURL(t *testing.T) {
	_, _, err := ParseRepoURL("https://gitlab.com/" + project.GitHubOrgRepo + ".git")
	if err == nil {
		t.Error("expected error for non-GitHub URL")
	}
}
