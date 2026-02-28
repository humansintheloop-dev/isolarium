package nono

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

func TestCreateChecksNonoAndWritesMetadata(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("nono", "--version").Returns("nono 1.0.0\n")

	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := creator.Create("my-sandbox", "/home/user/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()

	metadataPath := filepath.Join(metadataDir, "my-sandbox", "nono", "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read metadata: %v", err)
	}

	var meta NonoMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("failed to parse metadata: %v", err)
	}

	if meta.Type != "nono" {
		t.Errorf("expected type %q, got %q", "nono", meta.Type)
	}
	if meta.WorkDirectory != "/home/user/project" {
		t.Errorf("expected work directory %q, got %q", "/home/user/project", meta.WorkDirectory)
	}
}

func TestCreateFailsWhenNonoNotInstalled(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("nono", "--version").Fails(&fakeExecError{msg: "nono not found"})

	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := creator.Create("my-sandbox", "/home/user/project")
	if err == nil {
		t.Fatal("expected error when nono is not installed")
	}

	expectedMessage := "nono is not installed. Install nono to use sandbox mode."
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error message to contain %q, got %q", expectedMessage, err.Error())
	}
}

type fakeExecError struct {
	msg string
}

func (e *fakeExecError) Error() string {
	return e.msg
}
