package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	return filepath.Join(cwd, "isolarium")
}

func runHelpOutput(t *testing.T, binaryPath string) string {
	t.Helper()
	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}
	return string(output)
}

func assertOutputContains(t *testing.T, output string, expected ...string) {
	t.Helper()
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q, got: %s", s, output)
		}
	}
}

func limactlOutput(t *testing.T) (string, bool) {
	t.Helper()
	checkCmd := exec.Command("limactl", "list", "--json")
	out, err := checkCmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func limactlVMExists(output string) bool {
	return strings.Contains(output, `"name":"isolarium"`) ||
		strings.Contains(output, `"name": "isolarium"`)
}

func TestStatusCommand_ShowsNoEnvironmentsWhenNoneExist(t *testing.T) {
	buildBinary(t)

	cmd := exec.Command("./isolarium", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status command failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "No environments found") &&
		!strings.Contains(outputStr, "NAME") {
		t.Errorf("expected output to contain 'No environments found' or table header, got: %s", outputStr)
	}
}

func createTestStatusMetadata(t *testing.T) {
	t.Helper()
	baseDir := filepath.Join(os.Getenv("HOME"), ".isolarium", "test-status-env", "vm")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("failed to create test metadata dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "metadata.json"), []byte(`{"owner":"","repo":"","branch":""}`), 0644); err != nil {
		t.Fatalf("failed to write test metadata: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".isolarium", "test-status-env")) })
}

func TestStatusCommand_ShowsTableHeaderWhenEnvironmentsExist(t *testing.T) {
	createTestStatusMetadata(t)
	buildBinary(t)

	cmd := exec.Command("./isolarium", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status command failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	assertOutputContains(t, outputStr, "NAME", "TYPE", "STATE", "test-status-env")
}

func TestRunCommand_HasCopySessionFlag(t *testing.T) {
	output := runHelpOutput(t, buildBinary(t))
	assertOutputContains(t, output, "--copy-session")
}

func TestRunCommand_HasFreshLoginFlag(t *testing.T) {
	output := runHelpOutput(t, buildBinary(t))
	assertOutputContains(t, output, "--fresh-login")
}

func TestRunCommand_FreshLoginAndCopySessionMutuallyExclusive(t *testing.T) {
	binaryPath := buildBinary(t)

	cmd := exec.Command(binaryPath, "run", "--fresh-login", "--copy-session", "--", "echo", "hello")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected error when both --fresh-login and --copy-session are set, got: %s", output)
	}
	assertOutputContains(t, string(output), "mutually exclusive")
}

func TestRunCommand_UsageShowsCommandSyntax(t *testing.T) {
	output := runHelpOutput(t, buildBinary(t))
	assertOutputContains(t, output, "[flags] -- command")
}

func TestRunCommand_FailsWithNoCommand(t *testing.T) {
	binaryPath := buildBinary(t)

	cmd := exec.Command(binaryPath, "run", "--copy-session=false")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected run with no command to fail, got: %s", output)
	}
	assertOutputContains(t, string(output), "no command specified")
}

func TestRunCommand_HasInteractiveFlag(t *testing.T) {
	output := runHelpOutput(t, buildBinary(t))
	assertOutputContains(t, output, "--interactive", "-i")
}

func TestRunCommand_CreatesVMWhenNoneExists(t *testing.T) {
	if _, ok := limactlOutput(t); !ok {
		t.Skip("limactl not available, skipping test")
	}

	binaryPath := buildBinary(t)

	// Run from a non-git directory with --no-gh-token so the command reaches
	// the VM execution stage, proving it went through the create-or-start path
	// instead of erroring about no VM.
	tmpDir := t.TempDir()
	cmd := exec.Command(binaryPath, "run", "--name", "test-novm", "--copy-session=false", "--no-gh-token", "--env-file", "/dev/null", "--", "echo", "hello")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()

	outputStr := string(output)
	if err == nil {
		// Command succeeded — the create path worked and the VM ran the command
		return
	}

	if containsAcceptableVMError(outputStr) {
		return
	}

	t.Errorf("expected command to reach VM execution stage, got: %s", outputStr)
}

func TestCreateCommand_FailsWhenNotInGitRepo(t *testing.T) {
	binaryPath := buildBinary(t)
	tmpDir := t.TempDir()

	cmd := exec.Command(binaryPath, "create")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected create command to fail in non-git directory, but it succeeded. Output: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "not a git repository") {
		t.Errorf("expected error message to contain 'not a git repository', got: %s", outputStr)
	}
}

func TestCreateCommand_FailsWhenVMAlreadyExists(t *testing.T) {
	out, ok := limactlOutput(t)
	if !ok {
		t.Skip("limactl not available, skipping VM already-exists test")
	}
	if !limactlVMExists(out) {
		t.Skip("no isolarium VM exists, skipping VM already-exists test")
	}

	binaryPath := buildBinary(t)

	cmd := exec.Command(binaryPath, "create")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected create to fail when VM exists, got: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "already exists") {
		t.Errorf("expected error about VM already existing, got: %s", outputStr)
	}
}

func TestDestroyCommand_SucceedsWhenNoVMExists(t *testing.T) {
	out, ok := limactlOutput(t)
	if !ok {
		t.Skip("limactl not available, skipping destroy idempotent test")
	}
	if limactlVMExists(out) {
		t.Skip("isolarium VM exists, skipping no-VM destroy test")
	}

	binaryPath := buildBinary(t)

	cmd := exec.Command(binaryPath, "destroy")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("expected destroy to succeed when no VM exists, got error: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "no VM to destroy") {
		t.Errorf("expected 'no VM to destroy' message, got: %s", outputStr)
	}
}

func requireIsolariumVM(t *testing.T) {
	t.Helper()
	out, ok := limactlOutput(t)
	if !ok {
		t.Skip("limactl not available, skipping signal test")
	}
	if !limactlVMExists(out) {
		t.Skip("no isolarium VM exists, skipping signal test")
	}
}

func waitForProcessExit(t *testing.T, cmd *exec.Cmd, timeout time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		t.Fatal("process did not terminate within timeout after SIGINT")
	}
}

func TestRunCommand_TerminatesOnSIGINT(t *testing.T) {
	requireIsolariumVM(t)
	binaryPath := buildBinary(t)

	cmd := exec.Command(binaryPath, "run", "--copy-session=false", "--", "sleep", "3600")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start run command: %v", err)
	}

	time.Sleep(2 * time.Second)

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	waitForProcessExit(t, cmd, 10*time.Second)
}

func containsAcceptableVMError(output string) bool {
	return strings.Contains(output, "No such file or directory") ||
		strings.Contains(output, "not a git repository")
}
