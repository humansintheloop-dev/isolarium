//go:build integration

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/command"
)

const testContainerName = "isolarium-integration-test"
const testImageTag = "isolarium-integration-test:latest"

func TestContainerLifecycle_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	cleanupContainer(runner)

	workDir := t.TempDir()
	writeMarkerFile(t, workDir)
	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	metadataDir := t.TempDir()

	createContainer(t, runner, metadataDir, workDir, contextDir)
	verifyContainerRunning(t, runner)
	verifyMountedFilesVisible(t, runner)
	destroyContainer(t, runner, metadataDir)
	verifyContainerGone(t, runner)
}

func TestContainerSecurityFlags_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	cleanupContainer(runner)

	workDir := t.TempDir()
	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	metadataDir := t.TempDir()

	createContainer(t, runner, metadataDir, workDir, contextDir)
	defer cleanupContainer(runner)

	verifyNonRootUser(t, runner)
	verifyCapabilitiesDropped(t, runner)
}

func writeMarkerFile(t *testing.T, workDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workDir, "marker.txt"), []byte("integration-test"), 0644); err != nil {
		t.Fatalf("failed to write marker file: %v", err)
	}
}

func buildDockerContext(t *testing.T) string {
	t.Helper()
	dir, err := WriteDockerTempfile()
	if err != nil {
		t.Fatalf("failed to write Docker context: %v", err)
	}
	return dir
}

func createContainer(t *testing.T, runner command.ExecRunner, metadataDir, workDir, contextDir string) {
	t.Helper()
	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
		ImageTag:    testImageTag,
	}
	if err := creator.Create(testContainerName, workDir, contextDir); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
}

func verifyContainerRunning(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	checker := &StateChecker{Runner: runner}
	state := checker.GetState(testContainerName)
	if state != "running" {
		t.Fatalf("expected container state 'running', got %q", state)
	}
}

func verifyMountedFilesVisible(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	args := BuildExecCommand(testContainerName, nil, []string{"cat", "/home/isolarium/repo/marker.txt"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("exec cat marker.txt failed: %v", err)
	}
	if !strings.Contains(string(output), "integration-test") {
		t.Errorf("expected marker file content 'integration-test', got %q", string(output))
	}
}

func destroyContainer(t *testing.T, runner command.ExecRunner, metadataDir string) {
	t.Helper()
	destroyer := &Destroyer{
		Runner:      runner,
		MetadataDir: metadataDir,
	}
	if err := destroyer.Destroy(testContainerName); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
}

func verifyContainerGone(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	checker := &StateChecker{Runner: runner}
	state := checker.GetState(testContainerName)
	if state != "none" {
		t.Errorf("expected container state 'none' after destroy, got %q", state)
	}
}

func verifyNonRootUser(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	args := BuildExecCommand(testContainerName, nil, []string{"whoami"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("exec whoami failed: %v", err)
	}
	user := strings.TrimSpace(string(output))
	if user == "root" {
		t.Error("container is running as root, expected non-root user")
	}
}

func verifyCapabilitiesDropped(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	args := BuildExecCommand(testContainerName, nil, []string{"cat", "/proc/1/status"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("exec cat /proc/1/status failed: %v", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			capValue := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
			if capValue != "0000000000000000" {
				t.Errorf("expected all capabilities dropped (CapEff: 0000000000000000), got CapEff: %s", capValue)
			}
			return
		}
	}
	t.Error("CapEff line not found in /proc/1/status")
}

func cleanupContainer(runner command.ExecRunner) {
	runner.Run("docker", "rm", "-f", testContainerName)
}
