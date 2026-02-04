package lima

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRepoMetadata_JSON(t *testing.T) {
	clonedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	meta := RepoMetadata{
		Owner:    "cer",
		Repo:     "isolarium",
		Branch:   "main",
		ClonedAt: clonedAt,
	}

	// Test serialization
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Test deserialization
	var parsed RepoMetadata
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Owner != "cer" {
		t.Errorf("expected owner 'cer', got '%s'", parsed.Owner)
	}
	if parsed.Repo != "isolarium" {
		t.Errorf("expected repo 'isolarium', got '%s'", parsed.Repo)
	}
	if parsed.Branch != "main" {
		t.Errorf("expected branch 'main', got '%s'", parsed.Branch)
	}
	if !parsed.ClonedAt.Equal(clonedAt) {
		t.Errorf("expected cloned_at %v, got %v", clonedAt, parsed.ClonedAt)
	}
}

func TestBuildWriteMetadataCommand(t *testing.T) {
	jsonData := `{"owner":"cer","repo":"isolarium"}`
	cmd := BuildWriteMetadataCommand(jsonData)

	// Should create .isolarium directory and write the file
	if cmd[0] != "limactl" || cmd[1] != "shell" || cmd[2] != vmName {
		t.Errorf("unexpected command prefix: %v", cmd[:3])
	}
	if cmd[3] != "--" {
		t.Errorf("expected '--', got %q", cmd[3])
	}
	// The rest should be a bash command that creates dir and writes file
	bashCmd := cmd[4]
	if bashCmd != "bash" {
		t.Errorf("expected 'bash', got %q", bashCmd)
	}
}

func TestBuildReadMetadataCommand(t *testing.T) {
	cmd := BuildReadMetadataCommand()

	expected := []string{"limactl", "shell", vmName, "--", "cat", metadataPath}
	if len(cmd) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(cmd))
	}
	for i, arg := range expected {
		if cmd[i] != arg {
			t.Errorf("arg %d: expected %q, got %q", i, arg, cmd[i])
		}
	}
}
