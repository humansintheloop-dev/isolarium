package backend

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/command"
)

func TestNonoBackendCreateDelegatesToNonoCreator(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("nono", "--version").Returns("nono 1.0.0\n")

	b := &NonoBackend{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := b.Create("my-sandbox", CreateOptions{WorkDirectory: "/home/user/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()

	metadataPath := filepath.Join(metadataDir, "my-sandbox", "nono", "metadata.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Fatal("expected metadata file to exist")
	}
}

func TestNonoBackendCreateFailsWhenNonoNotInstalled(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("nono", "--version").Fails(&fakeError{msg: "nono not found"})

	b := &NonoBackend{
		Runner:      runner,
		MetadataDir: metadataDir,
	}

	err := b.Create("my-sandbox", CreateOptions{WorkDirectory: "/home/user/project"})
	if err == nil {
		t.Fatal("expected error when nono is not installed")
	}

	expectedMessage := "nono is not installed. Install nono to use sandbox mode."
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error message to contain %q, got %q", expectedMessage, err.Error())
	}
}

func TestNonoBackendGetStateReturnsConfiguredWhenMetadataDirExists(t *testing.T) {
	metadataDir := t.TempDir()
	nonoDir := filepath.Join(metadataDir, "my-sandbox", "nono")
	if err := os.MkdirAll(nonoDir, 0755); err != nil {
		t.Fatalf("failed to create nono dir: %v", err)
	}

	b := &NonoBackend{
		MetadataDir: metadataDir,
	}

	state := b.GetState("my-sandbox")
	if state != "configured" {
		t.Errorf("expected %q, got %q", "configured", state)
	}
}

func TestNonoBackendGetStateReturnsNoneWhenMetadataDirDoesNotExist(t *testing.T) {
	metadataDir := t.TempDir()

	b := &NonoBackend{
		MetadataDir: metadataDir,
	}

	state := b.GetState("my-sandbox")
	if state != "none" {
		t.Errorf("expected %q, got %q", "none", state)
	}
}

func TestNonoBackendCopyCredentialsReturnsNil(t *testing.T) {
	b := &NonoBackend{}

	err := b.CopyCredentials("my-sandbox", `{"token":"abc123"}`)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestNonoBackendDestroyReturnsUnsupportedOperationError(t *testing.T) {
	b := &NonoBackend{}

	err := b.Destroy("my-sandbox")
	if err == nil {
		t.Fatal("expected error from Destroy")
	}

	var unsupported *UnsupportedOperationError
	if !errors.As(err, &unsupported) {
		t.Errorf("expected UnsupportedOperationError, got %T: %v", err, err)
	}
}

func TestNonoBackendExecDelegatesToExecFunc(t *testing.T) {
	var calledName string
	var calledEnvVars map[string]string
	var calledArgs []string

	b := &NonoBackend{
		ExecFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			calledName = name
			calledEnvVars = envVars
			calledArgs = args
			return 0, nil
		},
	}

	exitCode, err := b.Exec("my-sandbox", map[string]string{"FOO": "bar"}, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if calledName != "my-sandbox" {
		t.Errorf("expected name 'my-sandbox', got '%s'", calledName)
	}
	if calledEnvVars["FOO"] != "bar" {
		t.Errorf("expected envVars to contain FOO=bar, got %v", calledEnvVars)
	}
	if len(calledArgs) != 2 || calledArgs[0] != "echo" || calledArgs[1] != "hello" {
		t.Errorf("expected args [echo hello], got %v", calledArgs)
	}
}

func TestNonoBackendExecPropagatesExitCode(t *testing.T) {
	b := &NonoBackend{
		ExecFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			return 42, nil
		},
	}

	exitCode, err := b.Exec("my-sandbox", nil, []string{"false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestNonoBackendExecInteractiveDelegatesToExecInteractiveFunc(t *testing.T) {
	var calledName string
	var calledEnvVars map[string]string
	var calledArgs []string

	b := &NonoBackend{
		ExecInteractiveFunc: func(name string, envVars map[string]string, args []string) (int, error) {
			calledName = name
			calledEnvVars = envVars
			calledArgs = args
			return 0, nil
		},
	}

	exitCode, err := b.ExecInteractive("my-sandbox", map[string]string{"FOO": "bar"}, []string{"claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if calledName != "my-sandbox" {
		t.Errorf("expected name 'my-sandbox', got '%s'", calledName)
	}
	if calledEnvVars["FOO"] != "bar" {
		t.Errorf("expected envVars to contain FOO=bar, got %v", calledEnvVars)
	}
	if len(calledArgs) != 1 || calledArgs[0] != "claude" {
		t.Errorf("expected args [claude], got %v", calledArgs)
	}
}

func TestNonoBackendOpenShellReturnsUnsupportedOperationError(t *testing.T) {
	b := &NonoBackend{}

	_, err := b.OpenShell("my-sandbox", nil)
	if err == nil {
		t.Fatal("expected error from OpenShell")
	}

	var unsupported *UnsupportedOperationError
	if !errors.As(err, &unsupported) {
		t.Errorf("expected UnsupportedOperationError, got %T: %v", err, err)
	}
}

type fakeError struct {
	msg string
}

func (e *fakeError) Error() string {
	return e.msg
}
