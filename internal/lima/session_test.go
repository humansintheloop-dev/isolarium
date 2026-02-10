package lima

import (
	"testing"
)

func TestBuildWriteCredentialsCommand(t *testing.T) {
	args := BuildWriteCredentialsCommand("isolarium")

	expected := []string{"limactl", "shell", "isolarium", "--", "bash", "-c", "cat > ~/.claude/.credentials.json"}
	if len(args) != len(expected) {
		t.Errorf("BuildWriteCredentialsCommand() = %v, want %v", args, expected)
		return
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("BuildWriteCredentialsCommand()[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestBuildCreateClaudeDirCommand(t *testing.T) {
	args := BuildCreateClaudeDirCommand("isolarium")

	expected := []string{"limactl", "shell", "isolarium", "--", "bash", "-c", "mkdir -p ~/.claude"}
	if len(args) != len(expected) {
		t.Errorf("BuildCreateClaudeDirCommand() = %v, want %v", args, expected)
		return
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("BuildCreateClaudeDirCommand()[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestBuildChmodCredentialsCommand(t *testing.T) {
	args := BuildChmodCredentialsCommand("isolarium")

	expected := []string{"limactl", "shell", "isolarium", "--", "bash", "-c", "chmod 600 ~/.claude/.credentials.json"}
	if len(args) != len(expected) {
		t.Errorf("BuildChmodCredentialsCommand() = %v, want %v", args, expected)
		return
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("BuildChmodCredentialsCommand()[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}
