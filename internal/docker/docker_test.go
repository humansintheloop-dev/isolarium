package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCheckDockerCommandProducesDockerInfoArgs(t *testing.T) {
	args := BuildCheckDockerCommand()
	expected := []string{"docker", "info"}
	if len(args) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Fatalf("expected args[%d] = %q, got %q", i, v, args[i])
		}
	}
}

func TestBuildImageCommandProducesCorrectDockerBuildArgs(t *testing.T) {
	args := BuildImageCommand("isolarium:latest", "/tmp/context")
	expected := []string{"docker", "build", "-t", "isolarium:latest", "/tmp/context"}
	if len(args) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Fatalf("expected args[%d] = %q, got %q", i, v, args[i])
		}
	}
}

func TestBuildRunCommandProducesCorrectDockerRunArgs(t *testing.T) {
	args := BuildRunCommand("my-container", "/home/user/project", "isolarium:latest")
	expected := []string{
		"docker", "run", "-d",
		"--name", "my-container",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"isolarium:latest",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Fatalf("expected args[%d] = %q, got %q", i, v, args[i])
		}
	}
}

func TestWriteDockerTempfileWritesEmbeddedDockerfileContent(t *testing.T) {
	dir, err := WriteDockerTempfile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.RemoveAll(dir)

	content, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}

	if len(content) == 0 {
		t.Fatal("Dockerfile is empty")
	}

	contentStr := string(content)
	if contentStr != dockerfileContent {
		t.Fatal("Dockerfile content does not match embedded content")
	}
}
