package docker

import (
	"fmt"
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
	args := BuildImageCommand("isolarium:latest", "/tmp/context", nil, nil)
	hostUID := fmt.Sprintf("HOST_UID=%d", os.Getuid())
	expected := []string{"docker", "build", "-t", "isolarium:latest", "--build-arg", hostUID, "/tmp/context"}
	if len(args) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Fatalf("expected args[%d] = %q, got %q", i, v, args[i])
		}
	}
}

func knownHostsVolume() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".ssh", "known_hosts") + ":/home/isolarium/.ssh/known_hosts:ro"
}

func TestBuildRunCommandProducesCorrectDockerRunArgs(t *testing.T) {
	args := BuildRunCommand("my-container", "/home/user/project", "isolarium:latest", nil)
	expected := []string{
		"docker", "run", "-d",
		"--name", "my-container",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"-v", knownHostsVolume(),
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

func TestBuildImageCommandIncludesBuildArgsForWorktree(t *testing.T) {
	wt := &WorktreeConfig{
		WorktreeHostPath: "/home/user/repos/myproject/worktrees/feature-branch",
		MainRepoHostPath: "/home/user/repos/myproject",
	}
	args := BuildImageCommand("isolarium:latest", "/tmp/context", wt, nil)
	hostUID := fmt.Sprintf("HOST_UID=%d", os.Getuid())
	expected := []string{
		"docker", "build", "-t", "isolarium:latest",
		"--build-arg", hostUID,
		"--build-arg", "WORKTREE_HOST_PATH=/home/user/repos/myproject/worktrees/feature-branch",
		"--build-arg", "MAIN_REPO_HOST_PATH=/home/user/repos/myproject",
		"/tmp/context",
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

func TestBuildRunCommandIncludesSecondVolumeForWorktree(t *testing.T) {
	wt := &WorktreeConfig{
		WorktreeHostPath: "/home/user/repos/myproject/worktrees/feature-branch",
		MainRepoHostPath: "/home/user/repos/myproject",
		MainRepoDir:      "/home/user/repos/myproject",
	}
	args := BuildRunCommand("my-container", "/home/user/project", "isolarium:latest", wt)
	expected := []string{
		"docker", "run", "-d",
		"--name", "my-container",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"-v", knownHostsVolume(),
		"-v", "/home/user/repos/myproject:/home/isolarium/main-repo",
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

func TestImageTagForContainerPrefixesWithIsolarium(t *testing.T) {
	tag := ImageTagForContainer("i2code-simple-banking")
	expected := "isolarium-i2code-simple-banking:latest"
	if tag != expected {
		t.Errorf("expected %q, got %q", expected, tag)
	}
}

func TestWriteDockerTempfileWritesEmbeddedDockerfileContent(t *testing.T) {
	dir, err := WriteDockerTempfile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

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
