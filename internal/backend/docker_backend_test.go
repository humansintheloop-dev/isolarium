package backend

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/git"
)

func hostUIDBuildArg() string {
	return fmt.Sprintf("HOST_UID=%d", os.Getuid())
}

func TestDockerBackendCreateDelegatesToDockerCreator(t *testing.T) {
	metadataDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", "--build-arg", hostUIDBuildArg(), contextDir).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"isolarium:latest",
	).Returns("container-id\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	err := b.Create("my-env", CreateOptions{WorkDirectory: "/home/user/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}

func TestDockerBackendCreateDetectsWorktreeAndPassesConfig(t *testing.T) {
	metadataDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	runner.OnCommand("docker", "build", "-t", "isolarium:latest",
		"--build-arg", hostUIDBuildArg(),
		"--build-arg", "WORKTREE_HOST_PATH=/home/user/worktree",
		"--build-arg", "MAIN_REPO_HOST_PATH=/home/user/main-repo",
		contextDir,
	).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/worktree:/home/isolarium/repo",
		"-v", "/home/user/main-repo:/home/isolarium/main-repo",
		"isolarium:latest",
	).Returns("container-id\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
		DetectWorktreeFunc: func(workDir string) (*git.WorktreeInfo, error) {
			return &git.WorktreeInfo{
				MainRepoDir: "/home/user/main-repo",
				WorktreeDir: "/home/user/worktree",
			}, nil
		},
	}

	err := b.Create("my-env", CreateOptions{WorkDirectory: "/home/user/worktree"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}

func TestDockerBackendCreateHandlesWorktreeDetectionError(t *testing.T) {
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    t.TempDir(),
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
		DetectWorktreeFunc: func(workDir string) (*git.WorktreeInfo, error) {
			return nil, fmt.Errorf("permission denied")
		},
	}

	err := b.Create("my-env", CreateOptions{WorkDirectory: "/home/user/project"})
	if err == nil {
		t.Fatal("expected error when worktree detection fails")
	}

	if !strings.Contains(err.Error(), "failed to detect git worktree") {
		t.Errorf("expected error to contain %q, got %q", "failed to detect git worktree", err.Error())
	}
}

func TestDockerBackendExecDelegatesToDockerExecCommand(t *testing.T) {
	var capturedName string
	var capturedEnvVars map[string]string
	var capturedArgs []string

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("running\n")

	b := &DockerBackend{
		Runner: runner,
		ExecFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			capturedName = name
			capturedEnvVars = envVars
			capturedArgs = args
			return 42, nil
		},
	}

	envVars := map[string]string{"GH_TOKEN": "ghs_abc123"}
	exitCode, err := b.Exec("my-container", envVars, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
	if capturedName != "my-container" {
		t.Errorf("expected name %q, got %q", "my-container", capturedName)
	}
	if capturedEnvVars["GH_TOKEN"] != "ghs_abc123" {
		t.Errorf("expected GH_TOKEN=ghs_abc123, got %v", capturedEnvVars)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "echo" || capturedArgs[1] != "hello" {
		t.Errorf("expected args [echo hello], got %v", capturedArgs)
	}
}

func TestDockerBackendExecInteractiveDelegatesToDockerExecInteractiveCommand(t *testing.T) {
	var capturedName string
	var capturedEnvVars map[string]string
	var capturedArgs []string

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("running\n")

	b := &DockerBackend{
		Runner: runner,
		ExecInteractiveFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			capturedName = name
			capturedEnvVars = envVars
			capturedArgs = args
			return 0, nil
		},
	}

	envVars := map[string]string{"GH_TOKEN": "ghs_abc123"}
	exitCode, err := b.ExecInteractive("my-container", envVars, []string{"bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if capturedName != "my-container" {
		t.Errorf("expected name %q, got %q", "my-container", capturedName)
	}
	if capturedEnvVars["GH_TOKEN"] != "ghs_abc123" {
		t.Errorf("expected GH_TOKEN=ghs_abc123, got %v", capturedEnvVars)
	}
	if len(capturedArgs) != 1 || capturedArgs[0] != "bash" {
		t.Errorf("expected args [bash], got %v", capturedArgs)
	}
}

func TestDockerBackendDestroyDelegatesToDockerDestroyer(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "rm", "-f", "my-env").Returns("")

	b := &DockerBackend{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := b.Destroy("my-env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}

func TestDockerBackendCopyCredentialsDelegatesToCopyCredentialsFunc(t *testing.T) {
	var capturedName string
	var capturedCredentials string

	b := &DockerBackend{
		CopyCredentialsFunc: func(name, credentials string) error {
			capturedName = name
			capturedCredentials = credentials
			return nil
		},
	}

	err := b.CopyCredentials("my-container", `{"token":"abc123"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedName != "my-container" {
		t.Errorf("expected name %q, got %q", "my-container", capturedName)
	}
	if capturedCredentials != `{"token":"abc123"}` {
		t.Errorf("expected credentials %q, got %q", `{"token":"abc123"}`, capturedCredentials)
	}
}

func TestDockerBackendExecReturnsErrorWhenContainerStopped(t *testing.T) {
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("exited\n")

	b := &DockerBackend{
		Runner: runner,
		ExecFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			t.Fatal("ExecFunc should not be called when container is stopped")
			return 0, nil
		},
	}

	_, err := b.Exec("my-container", nil, []string{"echo", "hello"})
	if err == nil {
		t.Fatal("expected error when container is stopped")
	}

	expectedMessage := "container 'my-container' is stopped, run 'isolarium create --type container' to recreate it"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error to contain %q, got %q", expectedMessage, err.Error())
	}
}

func TestDockerBackendExecInteractiveReturnsErrorWhenContainerStopped(t *testing.T) {
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("exited\n")

	b := &DockerBackend{
		Runner: runner,
		ExecInteractiveFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			t.Fatal("ExecInteractiveFunc should not be called when container is stopped")
			return 0, nil
		},
	}

	_, err := b.ExecInteractive("my-container", nil, []string{"bash"})
	if err == nil {
		t.Fatal("expected error when container is stopped")
	}

	expectedMessage := "container 'my-container' is stopped, run 'isolarium create --type container' to recreate it"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error to contain %q, got %q", expectedMessage, err.Error())
	}
}

func TestDockerBackendCreateLoadsPidYamlAndPreparesIsolationScripts(t *testing.T) {
	metadataDir := t.TempDir()
	workDir := t.TempDir()
	contextDir := t.TempDir()

	scriptsDir := workDir + "/scripts"
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptsDir+"/install.sh", []byte("#!/bin/bash\necho hi"), 0644); err != nil {
		t.Fatal(err)
	}

	pidYaml := `isolarium:
  container:
    isolation_scripts:
      - path: scripts/install.sh
        env:
          - MY_TOKEN
`
	if err := os.WriteFile(workDir+"/pid.yaml", []byte(pidYaml), 0644); err != nil {
		t.Fatal(err)
	}

	baseDockerfile := "FROM ubuntu:24.04\nUSER isolarium\nCMD [\"sleep\", \"infinity\"]\n"
	if err := os.WriteFile(contextDir+"/Dockerfile", []byte(baseDockerfile), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MY_TOKEN", "secret-value")

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	runner.OnCommand("docker", "build", "-t", "isolarium:latest",
		"--build-arg", hostUIDBuildArg(),
		"--build-arg", "MY_TOKEN=secret-value",
		contextDir,
	).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", workDir+":/home/isolarium/repo",
		"isolarium:latest",
	).Returns("container-id\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	err := b.Create("my-env", CreateOptions{WorkDirectory: workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}

func TestDockerBackendCreateFailsWhenDeclaredEnvVarMissing(t *testing.T) {
	workDir := t.TempDir()
	contextDir := t.TempDir()

	scriptsDir := workDir + "/scripts"
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptsDir+"/install.sh", []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	pidYaml := `isolarium:
  container:
    isolation_scripts:
      - path: scripts/install.sh
        env:
          - MISSING_VAR_XYZ
`
	if err := os.WriteFile(workDir+"/pid.yaml", []byte(pidYaml), 0644); err != nil {
		t.Fatal(err)
	}

	baseDockerfile := "FROM ubuntu:24.04\nCMD [\"sleep\", \"infinity\"]\n"
	if err := os.WriteFile(contextDir+"/Dockerfile", []byte(baseDockerfile), 0644); err != nil {
		t.Fatal(err)
	}

	b := &DockerBackend{
		Runner:         command.NewFakeRunner(t),
		MetadataDir:    t.TempDir(),
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	err := b.Create("my-env", CreateOptions{WorkDirectory: workDir})
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "MISSING_VAR_XYZ") {
		t.Errorf("expected error to mention MISSING_VAR_XYZ, got: %v", err)
	}
}

func TestDockerBackendGetStateDelegatesToDockerStateChecker(t *testing.T) {
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-env").Returns("running\n")

	b := &DockerBackend{
		Runner: runner,
	}

	state := b.GetState("my-env")
	if state != "running" {
		t.Errorf("expected %q, got %q", "running", state)
	}

	runner.VerifyExecuted()
}
