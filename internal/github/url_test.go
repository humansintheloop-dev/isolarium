package github

import (
	"testing"
)

func TestParseRepoURL_HTTPS(t *testing.T) {
	owner, repo, err := ParseRepoURL("https://github.com/cer/isolarium.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "cer" {
		t.Errorf("expected owner 'cer', got '%s'", owner)
	}
	if repo != "isolarium" {
		t.Errorf("expected repo 'isolarium', got '%s'", repo)
	}
}

func TestParseRepoURL_HTTPSWithoutGitSuffix(t *testing.T) {
	owner, repo, err := ParseRepoURL("https://github.com/cer/isolarium")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "cer" {
		t.Errorf("expected owner 'cer', got '%s'", owner)
	}
	if repo != "isolarium" {
		t.Errorf("expected repo 'isolarium', got '%s'", repo)
	}
}

func TestParseRepoURL_SSH(t *testing.T) {
	owner, repo, err := ParseRepoURL("git@github.com:cer/isolarium.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "cer" {
		t.Errorf("expected owner 'cer', got '%s'", owner)
	}
	if repo != "isolarium" {
		t.Errorf("expected repo 'isolarium', got '%s'", repo)
	}
}

func TestParseRepoURL_SSHWithoutGitSuffix(t *testing.T) {
	owner, repo, err := ParseRepoURL("git@github.com:cer/isolarium")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "cer" {
		t.Errorf("expected owner 'cer', got '%s'", owner)
	}
	if repo != "isolarium" {
		t.Errorf("expected repo 'isolarium', got '%s'", repo)
	}
}

func TestParseRepoURL_InvalidURL(t *testing.T) {
	_, _, err := ParseRepoURL("not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestParseRepoURL_NonGitHubURL(t *testing.T) {
	_, _, err := ParseRepoURL("https://gitlab.com/cer/isolarium.git")
	if err == nil {
		t.Error("expected error for non-GitHub URL")
	}
}
