package lima

import (
	"testing"
)

func TestBuildCloneURL_WithToken(t *testing.T) {
	url := BuildCloneURL("https://github.com/cer/isolarium.git", "ghs_token123")
	expected := "https://x-access-token:ghs_token123@github.com/cer/isolarium.git"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildCloneURL_WithoutToken(t *testing.T) {
	url := BuildCloneURL("https://github.com/cer/isolarium.git", "")
	expected := "https://github.com/cer/isolarium.git"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildCloneURL_SSHConvertsToHTTPS(t *testing.T) {
	url := BuildCloneURL("git@github.com:cer/isolarium.git", "ghs_token123")
	expected := "https://x-access-token:ghs_token123@github.com/cer/isolarium.git"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildCloneURL_SSHConvertsToHTTPS_NoToken(t *testing.T) {
	url := BuildCloneURL("git@github.com:cer/isolarium.git", "")
	expected := "https://github.com/cer/isolarium.git"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildCloneCommand(t *testing.T) {
	cmd := BuildCloneCommand("https://github.com/cer/isolarium.git", "main")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"git", "clone", "--branch", "main", "https://github.com/cer/isolarium.git", "repo",
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
	cmd := BuildWorkflowToolsCloneCommand("ghs_token123")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"git", "clone",
		"https://x-access-token:ghs_token123@github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git",
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
	cmd := BuildWorkflowToolsCloneCommand("")
	expected := []string{
		"limactl", "shell", "isolarium", "--",
		"git", "clone",
		"https://github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git",
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

func TestBuildInstallPluginCommand(t *testing.T) {
	cmd := BuildInstallPluginCommand()
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
	cmd := BuildInstallI2CodeCommand()
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
