//go:build integration

package nono

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

const testEnvName = "isolarium-nono-integration-test"

func TestNonoLifecycle_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	metadataDir := t.TempDir()
	workDir := t.TempDir()

	createNonoEnvironment(t, runner, metadataDir, workDir)
	verifyMetadataWritten(t, metadataDir)
	verifyRunEchoProducesOutput(t, workDir)
	destroyNonoEnvironment(t, metadataDir)
	verifyMetadataRemoved(t, metadataDir)
}

func TestNonoExecExitCode_Integration(t *testing.T) {
	workDir := t.TempDir()

	verifyExitCodePropagated(t, workDir, 42)
}

func createNonoEnvironment(t *testing.T, runner command.ExecRunner, metadataDir, workDir string) {
	t.Helper()
	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
	}
	if err := creator.Create(testEnvName, workDir); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
}

func verifyMetadataWritten(t *testing.T, metadataDir string) {
	t.Helper()
	store := NewMetadataStore(metadataDir, testEnvName)
	meta, err := store.Read()
	if err != nil {
		t.Fatalf("failed to read metadata: %v", err)
	}
	if meta.Type != "nono" {
		t.Errorf("expected metadata type 'nono', got %q", meta.Type)
	}
}

func verifyRunEchoProducesOutput(t *testing.T, workDir string) {
	t.Helper()
	cmdArgs := BuildRunCommand([]string{"echo", "hello"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("nono run echo hello failed: %v", err)
	}
	if strings.TrimSpace(string(output)) != "hello" {
		t.Errorf("expected output 'hello', got %q", strings.TrimSpace(string(output)))
	}
}

func destroyNonoEnvironment(t *testing.T, metadataDir string) {
	t.Helper()
	destroyer := &Destroyer{
		MetadataDir: metadataDir,
	}
	if err := destroyer.Destroy(testEnvName); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
}

func verifyMetadataRemoved(t *testing.T, metadataDir string) {
	t.Helper()
	store := NewMetadataStore(metadataDir, testEnvName)
	_, err := store.Read()
	if err == nil {
		t.Error("expected metadata to be removed after destroy, but it still exists")
	}
}

func verifyExitCodePropagated(t *testing.T, workDir string, expectedCode int) {
	t.Helper()
	cmdArgs := BuildRunCommand([]string{"sh", "-c", "exit 42"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code, but command succeeded")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != expectedCode {
		t.Errorf("expected exit code %d, got %d", expectedCode, exitErr.ExitCode())
	}
}
