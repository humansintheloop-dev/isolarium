package lima

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadMetadata_RoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	store := NewMetadataStore(baseDir, "testvm")

	if err := store.Write("cer", "isolarium", "main"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	meta, err := store.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if meta.Owner != "cer" {
		t.Errorf("expected owner 'cer', got %q", meta.Owner)
	}
	if meta.Repo != "isolarium" {
		t.Errorf("expected repo 'isolarium', got %q", meta.Repo)
	}
	if meta.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", meta.Branch)
	}
	if meta.ClonedAt.IsZero() {
		t.Error("expected ClonedAt to be set")
	}
}

func TestWriteMetadata_CreatesVMSubdirectory(t *testing.T) {
	baseDir := t.TempDir()
	store := NewMetadataStore(baseDir, "testvm")

	vmDir := filepath.Join(baseDir, "testvm", "vm")
	if _, err := os.Stat(vmDir); !os.IsNotExist(err) {
		t.Fatalf("expected vm directory to not exist yet")
	}

	if err := store.Write("cer", "isolarium", "main"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	info, err := os.Stat(vmDir)
	if err != nil {
		t.Fatalf("expected vm directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}

func TestWriteMetadata_FilePermissions(t *testing.T) {
	baseDir := t.TempDir()
	store := NewMetadataStore(baseDir, "testvm")

	if err := store.Write("cer", "isolarium", "main"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	filePath := filepath.Join(baseDir, "testvm", "vm", "repo.json")
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("expected permissions 0644, got %04o", perm)
	}
}

func TestReadMetadata_FileNotFound(t *testing.T) {
	baseDir := t.TempDir()
	store := NewMetadataStore(baseDir, "testvm")

	_, err := store.Read()
	if err == nil {
		t.Fatal("expected error when reading non-existent metadata")
	}
}

func TestCleanup_RemovesVMSubdirectory(t *testing.T) {
	baseDir := t.TempDir()
	store := NewMetadataStore(baseDir, "testvm")

	if err := store.Write("cer", "isolarium", "main"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := store.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	vmDir := filepath.Join(baseDir, "testvm", "vm")
	if _, err := os.Stat(vmDir); !os.IsNotExist(err) {
		t.Error("expected vm directory to be removed after cleanup")
	}
}

func TestCleanup_NoErrorWhenDirectoryMissing(t *testing.T) {
	baseDir := t.TempDir()
	store := NewMetadataStore(baseDir, "testvm")

	if err := store.Cleanup(); err != nil {
		t.Fatalf("Cleanup should not fail on missing directory: %v", err)
	}
}
