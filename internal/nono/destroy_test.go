package nono

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDestroyRemovesNonoMetadataDirectory(t *testing.T) {
	metadataDir := t.TempDir()

	nonoDir := filepath.Join(metadataDir, "my-sandbox", "nono")
	if err := os.MkdirAll(nonoDir, 0755); err != nil {
		t.Fatalf("failed to create nono dir: %v", err)
	}
	metadataFile := filepath.Join(nonoDir, "metadata.json")
	if err := os.WriteFile(metadataFile, []byte(`{"type":"nono"}`), 0644); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}

	destroyer := &Destroyer{
		MetadataDir: metadataDir,
	}

	err := destroyer.Destroy("my-sandbox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(nonoDir); !os.IsNotExist(err) {
		t.Error("expected nono metadata directory to be removed")
	}
}

func TestDestroySucceedsWhenMetadataDirectoryDoesNotExist(t *testing.T) {
	metadataDir := t.TempDir()

	destroyer := &Destroyer{
		MetadataDir: metadataDir,
	}

	err := destroyer.Destroy("nonexistent-sandbox")
	if err != nil {
		t.Fatalf("expected destroy to succeed for nonexistent metadata, got: %v", err)
	}
}
