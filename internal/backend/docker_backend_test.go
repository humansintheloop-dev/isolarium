package backend

import (
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/command"
)

func TestDockerBackendCreateDelegatesToDockerCreator(t *testing.T) {
	metadataDir := t.TempDir()
	contextDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	runner.OnCommand("docker", "build", "-t", "isolarium:latest", contextDir).Returns("")
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

	expectedMessage := "Container 'my-container' is stopped. Run 'isolarium create --type container' to recreate it."
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

	expectedMessage := "Container 'my-container' is stopped. Run 'isolarium create --type container' to recreate it."
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error to contain %q, got %q", expectedMessage, err.Error())
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
