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

func TestStatusCommand_OutputsValidVMState(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	// Run the status command
	cmd := exec.Command("./isolarium", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status command failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "VM: none") &&
		!strings.Contains(outputStr, "VM: running") &&
		!strings.Contains(outputStr, "VM: stopped") {
		t.Errorf("expected output to contain valid VM state, got: %s", outputStr)
	}
}

func TestStatusCommand_OutputsGitHubAppNotConfigured(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	// Run the status command
	cmd := exec.Command("./isolarium", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status command failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "GitHub App: not configured") {
		t.Errorf("expected output to contain 'GitHub App: not configured', got: %s", outputStr)
	}
}

func TestRunCommand_HasCopySessionFlag(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "--copy-session") {
		t.Errorf("expected 'run' command to have '--copy-session' flag, got: %s", outputStr)
	}
}

func TestRunCommand_HasFreshLoginFlag(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "--fresh-login") {
		t.Errorf("expected 'run' command to have '--fresh-login' flag, got: %s", outputStr)
	}
}

func TestRunCommand_FreshLoginAndCopySessionMutuallyExclusive(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--fresh-login", "--copy-session", "--", "echo", "hello")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected error when both --fresh-login and --copy-session are set, got: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "mutually exclusive") {
		t.Errorf("expected error about mutually exclusive flags, got: %s", outputStr)
	}
}

func TestRunCommand_UsageShowsCommandSyntax(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "[flags] -- command") {
		t.Errorf("expected usage to show '-- command' syntax, got: %s", outputStr)
	}
}

func TestRunCommand_FailsWithNoCommand(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--copy-session=false")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected run with no command to fail, got: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "no command specified") {
		t.Errorf("expected error about no command, got: %s", outputStr)
	}
}

func TestRunCommand_HasInteractiveFlag(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "--interactive") {
		t.Errorf("expected 'run' command to have '--interactive' flag, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "-i") {
		t.Errorf("expected 'run' command to have '-i' shorthand flag, got: %s", outputStr)
	}
}

func TestRunCommand_FailsWhenNoVMExists(t *testing.T) {
	checkCmd := exec.Command("limactl", "list", "--json")
	checkOutput, err := checkCmd.Output()
	if err != nil {
		t.Skip("limactl not available, skipping VM existence test")
	}
	if strings.Contains(string(checkOutput), `"name":"isolarium"`) ||
		strings.Contains(string(checkOutput), `"name": "isolarium"`) {
		t.Skip("isolarium VM already exists, skipping no-VM test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--copy-session=false", "--", "echo", "hello")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected run to fail when no VM exists, got: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "no VM exists") {
		t.Errorf("expected error about no VM, got: %s", outputStr)
	}
}

func TestCreateCommand_FailsWhenNotInGitRepo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	binaryPath := filepath.Join(cwd, "isolarium")
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
	checkCmd := exec.Command("limactl", "list", "--json")
	checkOutput, err := checkCmd.Output()
	if err != nil {
		t.Skip("limactl not available, skipping VM already-exists test")
	}
	if !strings.Contains(string(checkOutput), `"name":"isolarium"`) &&
		!strings.Contains(string(checkOutput), `"name": "isolarium"`) {
		t.Skip("no isolarium VM exists, skipping VM already-exists test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

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
	checkCmd := exec.Command("limactl", "list", "--json")
	checkOutput, err := checkCmd.Output()
	if err != nil {
		t.Skip("limactl not available, skipping destroy idempotent test")
	}
	if strings.Contains(string(checkOutput), `"name":"isolarium"`) ||
		strings.Contains(string(checkOutput), `"name": "isolarium"`) {
		t.Skip("isolarium VM exists, skipping no-VM destroy test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

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

func TestSSHCommand_Exists(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "ssh", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ssh --help failed: %v, output: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "interactive shell") {
		t.Errorf("expected ssh help to mention interactive shell, got: %s", outputStr)
	}
}

func TestSSHCommand_FailsWhenNoVMExists(t *testing.T) {
	checkCmd := exec.Command("limactl", "list", "--json")
	checkOutput, err := checkCmd.Output()
	if err != nil {
		t.Skip("limactl not available, skipping SSH no-VM test")
	}
	if strings.Contains(string(checkOutput), `"name":"isolarium"`) ||
		strings.Contains(string(checkOutput), `"name": "isolarium"`) {
		t.Skip("isolarium VM exists, skipping SSH no-VM test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "ssh")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected ssh to fail when no VM exists, got: %s", output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "no VM exists") {
		t.Errorf("expected error about no VM, got: %s", outputStr)
	}
}

func TestRunCommand_TerminatesOnSIGINT(t *testing.T) {
	checkCmd := exec.Command("limactl", "list", "--json")
	checkOutput, err := checkCmd.Output()
	if err != nil {
		t.Skip("limactl not available, skipping signal test")
	}
	if !strings.Contains(string(checkOutput), `"name":"isolarium"`) &&
		!strings.Contains(string(checkOutput), `"name": "isolarium"`) {
		t.Skip("no isolarium VM exists, skipping signal test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}
	binaryPath := filepath.Join(cwd, "isolarium")

	cmd := exec.Command(binaryPath, "run", "--copy-session=false", "--", "sleep", "3600")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start run command: %v", err)
	}

	time.Sleep(2 * time.Second)

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited - success
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not terminate within 10 seconds after SIGINT")
	}
}
