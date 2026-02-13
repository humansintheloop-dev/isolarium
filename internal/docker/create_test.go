package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/command"
)

func TestCreateChecksDockerAndBuildsImageAndStartsContainerAndWritesMetadata(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", metadataDir).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"isolarium:latest",
	).Returns("container-id-abc123\n")

	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
		ImageTag:    "isolarium:latest",
	}

	err := creator.Create("my-env", "/home/user/project", metadataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()

	metadataPath := filepath.Join(metadataDir, "my-env", "container", "metadata.json")
	data, err := os.ReadFile(metadataPath)
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
}

func TestCreateWithWorktreePassesBuildArgsAndSecondVolume(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	runner.OnCommand("docker", "build", "-t", "isolarium:latest",
		"--build-arg", "WORKTREE_HOST_PATH=/home/user/worktree",
		"--build-arg", "MAIN_REPO_HOST_PATH=/home/user/main-repo",
		metadataDir,
	).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/worktree:/home/isolarium/repo",
		"-v", "/home/user/main-repo:/home/isolarium/main-repo",
		"isolarium:latest",
	).Returns("container-id-abc123\n")

	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
		ImageTag:    "isolarium:latest",
		Worktree: &WorktreeConfig{
			WorktreeHostPath: "/home/user/worktree",
			MainRepoHostPath: "/home/user/main-repo",
			MainRepoDir:      "/home/user/main-repo",
		},
	}

	err := creator.Create("my-env", "/home/user/worktree", metadataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}

func TestCreateFailsWhenDockerNotAvailable(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Fails(
		&fakeExecError{msg: "docker not found"},
	)

	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
		ImageTag:    "isolarium:latest",
	}

	err := creator.Create("my-env", "/home/user/project", metadataDir)
	if err == nil {
		t.Fatal("expected error when Docker is not available")
	}

	expectedMessage := "Docker is not installed or not running. Install Docker Desktop (macOS) or Docker Engine (Linux) to use container mode"
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
