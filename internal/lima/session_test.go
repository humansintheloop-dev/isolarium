package lima

import (
	"testing"
)

func TestBuildCopyCredentialsCommand(t *testing.T) {
	tests := []struct {
		name            string
		credentialsPath string
		wantContains    []string
	}{
		{
			name:            "builds command with correct destination path",
			credentialsPath: "/Users/test/.claude/.credentials.json",
			wantContains: []string{
				"limactl",
				"copy",
				"/Users/test/.claude/.credentials.json",
				"isolarium:.claude/.credentials.json",
			},
		},
		{
			name:            "handles path with spaces",
			credentialsPath: "/Users/test user/.claude/.credentials.json",
			wantContains: []string{
				"limactl",
				"copy",
				"/Users/test user/.claude/.credentials.json",
				"isolarium:.claude/.credentials.json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := BuildCopyCredentialsCommand(tt.credentialsPath)

			// Check that all expected strings are present in the command args
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("BuildCopyCredentialsCommand() = %v, missing %q", args, want)
				}
			}
		})
	}
}

func TestBuildCreateClaudeDirCommand(t *testing.T) {
	args := BuildCreateClaudeDirCommand()

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
	args := BuildChmodCredentialsCommand()

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
