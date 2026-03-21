package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/docker"
	"github.com/humansintheloop-dev/isolarium/internal/git"
)

func hostUIDBuildArg() string {
	return fmt.Sprintf("HOST_UID=%d", os.Getuid())
}

func knownHostsVolume() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".ssh", "known_hosts") + ":/home/isolarium/.ssh/known_hosts:ro"
}

func stubI2CodeHeadSHA(runner *command.FakeRunner) {
	runner.OnCommand("git", "ls-remote", "https://github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git", "HEAD").Returns("aabbcc\tHEAD\n")
}

func i2CodeVersionBuildArg() string {
	return "I2CODE_VERSION=aabbcc"
}

func TestDockerBackendCreateDelegatesToDockerCreator(t *testing.T) {
	metadataDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	stubI2CodeHeadSHA(runner)
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", "--build-arg", hostUIDBuildArg(), "--build-arg", i2CodeVersionBuildArg(), contextDir).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"-v", knownHostsVolume(),
		"isolarium:latest",
	).Returns("container-id\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	err := b.Create(CreateOptions{Name: "my-env", WorkDirectory: "/home/user/project"})
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
	stubI2CodeHeadSHA(runner)
	runner.OnCommand("docker", "build", "-t", "isolarium:latest",
		"--build-arg", hostUIDBuildArg(),
		"--build-arg", "WORKTREE_HOST_PATH=/home/user/worktree",
		"--build-arg", "MAIN_REPO_HOST_PATH=/home/user/main-repo",
		"--build-arg", i2CodeVersionBuildArg(),
		contextDir,
	).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/worktree:/home/isolarium/repo",
		"-v", knownHostsVolume(),
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

	err := b.Create(CreateOptions{Name: "my-env", WorkDirectory: "/home/user/worktree"})
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

	err := b.Create(CreateOptions{Name: "my-env", WorkDirectory: "/home/user/project"})
	if err == nil {
		t.Fatal("expected error when worktree detection fails")
	}

	if !strings.Contains(err.Error(), "failed to detect git worktree") {
		t.Errorf("expected error to contain %q, got %q", "failed to detect git worktree", err.Error())
	}
}

type execCapture struct {
	name    string
	envVars map[string]string
	args    []string
}

func (c *execCapture) captureFunc(exitCode int) func(ExecRequest) (int, error) {
	return func(req ExecRequest) (int, error) {
		c.name = req.ContainerName
		c.envVars = req.EnvVars
		c.args = req.Args
		return exitCode, nil
	}
}

func runningContainerRunner(t *testing.T) *command.FakeRunner {
	t.Helper()
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("running\n")
	return runner
}

func assertExecCapture(t *testing.T, actual, expected execCapture) {
	t.Helper()
	if actual.name != expected.name {
		t.Errorf("expected name %q, got %q", expected.name, actual.name)
	}
	if !reflect.DeepEqual(actual.args, expected.args) {
		t.Errorf("expected args %v, got %v", expected.args, actual.args)
	}
	for key, val := range expected.envVars {
		if actual.envVars[key] != val {
			t.Errorf("expected %s=%s, got %v", key, val, actual.envVars)
		}
	}
}

func TestDockerBackendExecDelegatesToDockerExecCommand(t *testing.T) {
	capture := &execCapture{}
	b := &DockerBackend{
		Runner:   runningContainerRunner(t),
		ExecFunc: capture.captureFunc(42),
	}

	exitCode, err := b.Exec(ExecRequest{ContainerName: "my-container", EnvVars: map[string]string{"GH_TOKEN": "ghs_abc123"}, Args: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
	assertExecCapture(t, *capture, execCapture{name: "my-container", envVars: map[string]string{"GH_TOKEN": "ghs_abc123"}, args: []string{"echo", "hello"}})
}

func TestDockerBackendExecInteractiveDelegatesToDockerExecInteractiveCommand(t *testing.T) {
	capture := &execCapture{}
	b := &DockerBackend{
		Runner:              runningContainerRunner(t),
		ExecInteractiveFunc: capture.captureFunc(0),
	}

	exitCode, err := b.ExecInteractive(ExecRequest{ContainerName: "my-container", EnvVars: map[string]string{"GH_TOKEN": "ghs_abc123"}, Args: []string{"bash"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	assertExecCapture(t, *capture, execCapture{name: "my-container", envVars: map[string]string{"GH_TOKEN": "ghs_abc123"}, args: []string{"bash"}})
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

func stoppedContainerBackend(t *testing.T) *DockerBackend {
	t.Helper()
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("exited\n")
	return &DockerBackend{
		Runner: runner,
		ExecFunc: func(req ExecRequest) (int, error) {
			t.Fatal("ExecFunc should not be called when container is stopped")
			return 0, nil
		},
		ExecInteractiveFunc: func(req ExecRequest) (int, error) {
			t.Fatal("ExecInteractiveFunc should not be called when container is stopped")
			return 0, nil
		},
	}
}

func assertStoppedContainerError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error when container is stopped")
	}
	expectedMessage := "container 'my-container' is stopped, run 'isolarium create --type container' to recreate it"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error to contain %q, got %q", expectedMessage, err.Error())
	}
}

func TestDockerBackendExecReturnsErrorWhenContainerStopped(t *testing.T) {
	b := stoppedContainerBackend(t)
	_, err := b.Exec(ExecRequest{ContainerName: "my-container", Args: []string{"echo", "hello"}})
	assertStoppedContainerError(t, err)
}

func TestDockerBackendExecInteractiveReturnsErrorWhenContainerStopped(t *testing.T) {
	b := stoppedContainerBackend(t)
	_, err := b.ExecInteractive(ExecRequest{ContainerName: "my-container", Args: []string{"bash"}})
	assertStoppedContainerError(t, err)
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
    create:
      creation_scripts:
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
	stubI2CodeHeadSHA(runner)
	runner.OnCommand("docker", "build", "-t", "isolarium:latest",
		"--build-arg", hostUIDBuildArg(),
		"--build-arg", i2CodeVersionBuildArg(),
		"--build-arg", "MY_TOKEN=secret-value",
		contextDir,
	).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", workDir+":/home/isolarium/repo",
		"-v", knownHostsVolume(),
		"isolarium:latest",
	).Returns("container-id\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	err := b.Create(CreateOptions{Name: "my-env", WorkDirectory: workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}

type envScriptFixture struct {
	workDir   string
	runner    *command.FakeRunner
	execCalls []execCapture
	backend   *DockerBackend
}

func newEnvScriptFixture(t *testing.T) *envScriptFixture {
	t.Helper()
	metadataDir := t.TempDir()
	workDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	stubI2CodeHeadSHA(runner)
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", "--build-arg", hostUIDBuildArg(), "--build-arg", i2CodeVersionBuildArg(), contextDir).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", workDir+":/home/isolarium/repo",
		"-v", knownHostsVolume(),
		"isolarium:latest",
	).Returns("container-id\n")

	fix := &envScriptFixture{workDir: workDir, runner: runner}
	fix.backend = &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
		ExecFunc: func(req ExecRequest) (int, error) {
			fix.execCalls = append(fix.execCalls, execCapture{name: req.ContainerName, envVars: req.EnvVars, args: req.Args})
			return 0, nil
		},
	}
	return fix
}

func TestDockerBackendCreateRunsEnvScriptsInsideContainer(t *testing.T) {
	fix := newEnvScriptFixture(t)

	pidYaml := `isolarium:
  container:
    create:
      post_creation_scripts:
        env_scripts:
          - path: scripts/env-setup.sh
`
	if err := os.WriteFile(fix.workDir+"/pid.yaml", []byte(pidYaml), 0644); err != nil {
		t.Fatal(err)
	}

	err := fix.backend.Create(CreateOptions{Name: "my-env", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fix.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(fix.execCalls))
	}
	assertExecCapture(t, fix.execCalls[0], execCapture{
		name:    "my-env",
		envVars: map[string]string{"ISOLARIUM_NAME": "my-env", "ISOLARIUM_TYPE": "container"},
		args:    []string{"bash", "scripts/env-setup.sh"},
	})
	fix.runner.VerifyExecuted()
}

func TestDockerBackendCreatePassesEnvVarsToEnvScripts(t *testing.T) {
	fix := newEnvScriptFixture(t)

	pidYaml := `isolarium:
  container:
    create:
      post_creation_scripts:
        env_scripts:
          - path: scripts/install-plugin.sh
          - path: scripts/add-codescene-mcp.sh
            env:
              - CS_ACCESS_TOKEN
              - CS_ACE_ACCESS_TOKEN
`
	if err := os.WriteFile(fix.workDir+"/pid.yaml", []byte(pidYaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CS_ACCESS_TOKEN", "token123")
	t.Setenv("CS_ACE_ACCESS_TOKEN", "ace456")

	err := fix.backend.Create(CreateOptions{Name: "my-env", WorkDirectory: fix.workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fix.execCalls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(fix.execCalls))
	}

	if fix.execCalls[0].envVars["CS_ACCESS_TOKEN"] != "" {
		t.Errorf("install-plugin.sh should not receive CS_ACCESS_TOKEN, got %q", fix.execCalls[0].envVars["CS_ACCESS_TOKEN"])
	}
	assertExecCapture(t, fix.execCalls[1], execCapture{
		name:    "my-env",
		envVars: map[string]string{"CS_ACCESS_TOKEN": "token123", "CS_ACE_ACCESS_TOKEN": "ace456"},
		args:    []string{"bash", "scripts/add-codescene-mcp.sh"},
	})
	fix.runner.VerifyExecuted()
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
    create:
      creation_scripts:
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

	err := b.Create(CreateOptions{Name: "my-env", WorkDirectory: workDir})
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "MISSING_VAR_XYZ") {
		t.Errorf("expected error to mention MISSING_VAR_XYZ, got: %v", err)
	}
}

func TestRebuildIfChangedSkipsRecreateWhenImageUnchanged(t *testing.T) {
	metadataDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.Image}}", "my-env").Returns("sha256:abc123\n")
	stubI2CodeHeadSHA(runner)
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", "--build-arg", hostUIDBuildArg(), "--build-arg", i2CodeVersionBuildArg(), contextDir).Returns("")
	runner.OnCommand("docker", "inspect", "--format", "{{.Id}}", "isolarium:latest").Returns("sha256:abc123\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	changed, err := b.RebuildIfChanged(CreateOptions{Name: "my-env", WorkDirectory: "/home/user/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected no change when image IDs match")
	}

	runner.VerifyExecuted()
}

func TestRebuildIfChangedRecreatesContainerWhenImageChanged(t *testing.T) {
	metadataDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.Image}}", "my-env").Returns("sha256:old111\n")
	stubI2CodeHeadSHA(runner)
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", "--build-arg", hostUIDBuildArg(), "--build-arg", i2CodeVersionBuildArg(), contextDir).Returns("")
	runner.OnCommand("docker", "inspect", "--format", "{{.Id}}", "isolarium:latest").Returns("sha256:new222\n")
	runner.OnCommand("docker", "rm", "-f", "my-env").Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"-v", knownHostsVolume(),
		"isolarium:latest",
	).Returns("container-id\n")

	b := &DockerBackend{
		Runner:         runner,
		MetadataDir:    metadataDir,
		ImageTag:       "isolarium:latest",
		ContextDirFunc: func() (string, error) { return contextDir, nil },
	}

	changed, err := b.RebuildIfChanged(CreateOptions{Name: "my-env", WorkDirectory: "/home/user/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected change when image IDs differ")
	}

	runner.VerifyExecuted()
}

func backendWithStoredWorkDirectory(t *testing.T, storedDir string) *DockerBackend {
	t.Helper()
	metadataDir := t.TempDir()
	store := docker.NewMetadataStore(metadataDir, "my-env")
	if err := store.Write("container", storedDir); err != nil {
		t.Fatal(err)
	}
	return &DockerBackend{MetadataDir: metadataDir}
}

func TestWorkDirectoryChangedReturnsTrueWhenDirectoryDiffers(t *testing.T) {
	b := backendWithStoredWorkDirectory(t, "/old/path")

	if !b.WorkDirectoryChanged("my-env", "/new/path") {
		t.Error("expected WorkDirectoryChanged to return true when directories differ")
	}
}

func TestWorkDirectoryChangedReturnsFalseWhenDirectoryMatches(t *testing.T) {
	b := backendWithStoredWorkDirectory(t, "/same/path")

	if b.WorkDirectoryChanged("my-env", "/same/path") {
		t.Error("expected WorkDirectoryChanged to return false when directories match")
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
