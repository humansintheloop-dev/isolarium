package lima

import (
	"fmt"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/project"
)

func TestBuildCloneURL_WithToken(t *testing.T) {
	httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	url := BuildCloneURL(httpsURL, "ghs_token123")
	expected := fmt.Sprintf("https://x-access-token:ghs_token123@github.com/%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildCloneURL_WithoutToken(t *testing.T) {
	httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	url := BuildCloneURL(httpsURL, "")
	if url != httpsURL {
		t.Errorf("expected %q, got %q", httpsURL, url)
	}
}

func TestBuildCloneURL_SSHConvertsToHTTPS(t *testing.T) {
	sshURL := fmt.Sprintf("git@github.com:%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	url := BuildCloneURL(sshURL, "ghs_token123")
	expected := fmt.Sprintf("https://x-access-token:ghs_token123@github.com/%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildCloneURL_SSHPreservedWithoutToken(t *testing.T) {
	sshURL := fmt.Sprintf("git@github.com:%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	url := BuildCloneURL(sshURL, "")
	if url != sshURL {
		t.Errorf("expected %q, got %q", sshURL, url)
	}
}

func TestBuildCloneCommand(t *testing.T) {
	httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", project.GitHubOrg, project.GitHubRepo)
	cmd := BuildCloneCommand("isolarium", httpsURL, "main")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"git", "clone", "--branch", "main", httpsURL, "repo",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}

func TestBuildWorkflowToolsCloneCommand(t *testing.T) {
	cmd := BuildWorkflowToolsCloneCommand("isolarium", "ghs_token123")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"git", "clone",
		fmt.Sprintf("https://x-access-token:ghs_token123@github.com/%s.git", project.WorkflowToolsOrgRepo),
		"workflow-tools",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}

func TestBuildWorkflowToolsCloneCommand_NoToken(t *testing.T) {
	cmd := BuildWorkflowToolsCloneCommand("isolarium", "")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"git", "clone",
		fmt.Sprintf("https://github.com/%s.git", project.WorkflowToolsOrgRepo),
		"workflow-tools",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}

func TestBuildConfigureGitAuthorCommand(t *testing.T) {
	cmd := BuildConfigureGitAuthorCommand("isolarium", "dev+i2code@example.com", "Jane Dev")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"bash", "-c", "cd ~/repo && git config user.email 'dev+i2code@example.com' && git config user.name 'Jane Dev'",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}

func TestBuildInstallPluginCommand(t *testing.T) {
	cmd := BuildInstallPluginCommand("isolarium")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"bash", "-c", "cd ~/workflow-tools && ./install-plugin.sh",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}

func TestBuildInstallI2CodeCommand(t *testing.T) {
	cmd := BuildInstallI2CodeCommand("isolarium")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"bash", "-lc", "cd ~/workflow-tools && uv tool install -e .",
	}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}
