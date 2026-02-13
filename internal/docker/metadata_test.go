package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataStoreWritesCorrectJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir, "test-env")

	err := store.Write("container", "/home/user/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "test-env", "container", "metadata.json"))
	if err != nil {
		t.Fatalf("failed to read metadata: %v", err)
	}

	var meta DockerMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("failed to parse metadata: %v", err)
	}

	if meta.Type != "container" {
		t.Errorf("expected type %q, got %q", "container", meta.Type)
	}
	if meta.WorkDirectory != "/home/user/project" {
		t.Errorf("expected work directory %q, got %q", "/home/user/project", meta.WorkDirectory)
	}
	if meta.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestMetadataStoreReadReturnsWrittenMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir, "test-env")

	err := store.Write("container", "/home/user/project")
	if err != nil {
		t.Fatalf("unexpected error writing: %v", err)
	}

	meta, err := store.Read()
	if err != nil {
		t.Fatalf("unexpected error reading: %v", err)
	}

	if meta.Type != "container" {
		t.Errorf("expected type %q, got %q", "container", meta.Type)
	}
	if meta.WorkDirectory != "/home/user/project" {
		t.Errorf("expected work directory %q, got %q", "/home/user/project", meta.WorkDirectory)
	}
}

func TestMetadataStoreCleanupRemovesMetadataDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir, "test-env")

	err := store.Write("container", "/home/user/project")
	if err != nil {
		t.Fatalf("unexpected error writing: %v", err)
	}

	err = store.Cleanup()
	if err != nil {
		t.Fatalf("unexpected error cleaning up: %v", err)
	}

	metadataDir := filepath.Join(tmpDir, "test-env", "container")
	if _, err := os.Stat(metadataDir); !os.IsNotExist(err) {
		t.Error("expected metadata directory to be removed")
	}
}
