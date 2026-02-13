package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cer/isolarium/internal/command"
)

func TestBuildDestroyCommandProducesDockerRmForceArgs(t *testing.T) {
	args := BuildDestroyCommand("my-env")
	expected := []string{"docker", "rm", "-f", "my-env"}
	if len(args) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Fatalf("expected args[%d] = %q, got %q", i, v, args[i])
		}
	}
}

func TestDestroyRemovesContainerAndCleansMetadata(t *testing.T) {
	metadataDir := t.TempDir()

	store := NewMetadataStore(metadataDir, "my-env")
	if err := store.Write("container", "/home/user/project"); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "rm", "-f", "my-env").Returns("")

	destroyer := &Destroyer{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := destroyer.Destroy("my-env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()

	metadataPath := filepath.Join(metadataDir, "my-env", "container")
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Error("expected metadata directory to be removed")
	}
}

func TestDestroySucceedsWhenContainerDoesNotExist(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "rm", "-f", "nonexistent-env").Returns("")

	destroyer := &Destroyer{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := destroyer.Destroy("nonexistent-env")
	if err != nil {
		t.Fatalf("expected destroy to succeed for nonexistent container, got: %v", err)
	}

	runner.VerifyExecuted()
}
