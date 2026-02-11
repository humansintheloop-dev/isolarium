//go:build integration

package lima

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cer/isolarium/internal/github"
)

// Aliases for github package functions used in tests
var parseRepoURL = github.ParseRepoURL
var newTokenMinter = github.NewTokenMinter

// loadTestEnvFile loads .env.local from the project root
func loadTestEnvFile(t *testing.T) {
	t.Helper()
	envPath := findProjectRoot(t) + "/.env.local"
	file, err := os.Open(envPath)
	if err != nil {
		t.Logf(".env.local not found at %s, skipping", envPath)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}

// Integration tests for Lima VM management
// These tests require Lima to be installed and can take several minutes to run
// Run with: go test -tags=integration ./internal/lima/...

func TestZZZ_DestroyVM_Integration(t *testing.T) {
	ensureVMRunning(t)

	if err := DestroyVM(vmName); err != nil {
		t.Fatalf("DestroyVM failed: %v", err)
	}

	exists, err := VMExists(vmName)
	if err != nil {
		t.Fatalf("failed to check VM status: %v", err)
	}
	if exists {
		t.Error("VM still exists after destroy")
	}
}

// Task 8.3: Test DestroyVM is idempotent — runs after TestZZZ_DestroyVM_Integration
func TestZZZZ_DestroyVM_Idempotent_Integration(t *testing.T) {
	if err := DestroyVM(vmName); err != nil {
		t.Fatalf("first DestroyVM with no VM failed: %v", err)
	}

	if err := DestroyVM(vmName); err != nil {
		t.Fatalf("second DestroyVM with no VM failed: %v", err)
	}
}

func TestVMHasRequiredTools_Integration(t *testing.T) {
	ensureVMRunning(t)

	// Check for required tools (all should be in PATH via symlinks or direct install)
	tools := []string{"git", "node", "docker", "gh", "claude", "java"}
	for _, tool := range tools {
		cmd := vmShell("which", tool)
		if err := cmd.Run(); err != nil {
			t.Errorf("tool %s not found in VM", tool)
		}
	}

	// Check JAVA_HOME is set in /etc/environment
	cmd := vmShell("grep", "JAVA_HOME", "/etc/environment")
	if err := cmd.Run(); err != nil {
		t.Error("JAVA_HOME not set in /etc/environment")
	}
}

func TestCloneRepoWithToken_Integration(t *testing.T) {
	ensureVMRunning(t)

	// Get the expected branch from the host repo
	branchCmd := exec.Command("git", "branch", "--show-current")
	branchOutput, err := branchCmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	expectedBranch := strings.TrimSpace(string(branchOutput))

	// Get the expected remote URL
	remoteCmd := exec.Command("git", "remote", "get-url", "origin")
	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		t.Fatalf("failed to get remote URL: %v", err)
	}
	remoteURL := strings.TrimSpace(string(remoteOutput))
	t.Logf("Testing with repo: %s, branch: %s", remoteURL, expectedBranch)

	// Remove any existing repo directory
	vmShell("rm", "-rf", "repo").Run()

	// Load .env.local to get GitHub App credentials
	loadTestEnvFile(t)

	// Mint token if credentials are available
	var token string
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if appID != "" && privateKeyPath != "" {
		privateKeyBytes, err := os.ReadFile(privateKeyPath)
		if err != nil {
			t.Fatalf("failed to read private key: %v", err)
		}

		// Parse owner/repo from URL
		owner, repo, err := parseRepoURL(remoteURL)
		if err != nil {
			t.Fatalf("failed to parse repo URL: %v", err)
		}

		minter, err := newTokenMinter(appID, string(privateKeyBytes), "")
		if err != nil {
			t.Fatalf("failed to create token minter: %v", err)
		}

		token, err = minter.MintInstallationToken(owner, repo)
		if err != nil {
			t.Fatalf("failed to mint token: %v", err)
		}
		t.Log("Token minted successfully")
	} else {
		t.Fatal("GitHub App credentials not configured - set GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY_PATH in .env.local")
	}

	if err := CloneRepo(vmName, findProjectRoot(t), remoteURL, expectedBranch, token); err != nil {
		t.Fatalf("CloneRepo failed: %v", err)
	}

	// Verify repo was cloned by checking for go.mod
	cmd := vmShell("test", "-f", "repo/go.mod")
	if err := cmd.Run(); err != nil {
		t.Error("go.mod not found in cloned repo - repo was not cloned")
	}

	// Verify correct branch is checked out
	cmd = vmShell("git", "-C", "repo", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch in VM: %v", err)
	}
	actualBranch := strings.TrimSpace(string(output))
	if actualBranch != expectedBranch {
		t.Errorf("expected branch %q, got %q", expectedBranch, actualBranch)
	}

}

// findProjectRoot returns the project root directory
func findProjectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}


func TestCopyClaudeCredentials_Integration(t *testing.T) {
	// Try reading credentials from macOS Keychain first, fall back to file for CI
	credentials, err := readCredentialsFromKeychain()
	if err != nil {
		t.Logf("Keychain read failed (%v), falling back to CLAUDE_CREDENTIALS_PATH", err)
		loadTestEnvFile(t)
		credentialsPath := os.Getenv("CLAUDE_CREDENTIALS_PATH")
		if credentialsPath == "" {
			t.Fatal("no credentials available: Keychain failed and CLAUDE_CREDENTIALS_PATH not set")
		}
		data, err := os.ReadFile(credentialsPath)
		if err != nil {
			t.Fatalf("failed to read credentials file %s: %v", credentialsPath, err)
		}
		credentials = string(data)
	}

	ensureVMRunning(t)

	// Remove any existing credentials in VM first
	vmShell("bash", "-c", "rm -rf ~/.claude").Run()

	if err := CopyClaudeCredentials(vmName, credentials); err != nil {
		t.Fatalf("CopyClaudeCredentials failed: %v", err)
	}

	// Verify file exists in VM
	cmd := vmShell("test", "-f", ".claude/.credentials.json")
	if err := cmd.Run(); err != nil {
		t.Fatal("credentials file does not exist in VM")
	}

	// Verify permissions are 600
	cmd = vmShell("stat", "-c", "%a", ".claude/.credentials.json")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to check file permissions: %v", err)
	}
	perms := strings.TrimSpace(string(output))
	if perms != "600" {
		t.Errorf("expected permissions 600, got %s", perms)
	}

	// Run Claude inside the VM to verify credentials work
	// Have Claude create a file to prove it can actually do work
	cmd = vmShell("bash", "-c",
		"cd /tmp && claude --allowed-tools Write -p 'Create a Java hello world called HelloWorld.java'")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("claude command failed: %v\noutput: %s", err, output)
	}
	t.Logf("Claude response: %s", output)

	// Verify the file was created
	cmd = vmShell("test", "-f", "/tmp/HelloWorld.java")
	if err := cmd.Run(); err != nil {
		t.Fatal("Claude did not create HelloWorld.java")
	}

	// Verify the file contains valid Java code
	cmd = vmShell("cat", "/tmp/HelloWorld.java")
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("failed to read HelloWorld.java: %v", err)
	}
	content := string(output)
	if !strings.Contains(content, "class HelloWorld") {
		t.Errorf("HelloWorld.java does not contain expected class: %s", content)
	}
	if !strings.Contains(content, "public static void main") {
		t.Errorf("HelloWorld.java does not contain main method: %s", content)
	}
	t.Logf("HelloWorld.java content:\n%s", content)
}

func TestCloneWorkflowTools_Integration(t *testing.T) {
	ensureVMRunning(t)

	// Remove any existing workflow-tools directory
	vmShell("rm", "-rf", "workflow-tools").Run()

	if err := CloneWorkflowTools(vmName, ""); err != nil {
		t.Fatalf("CloneWorkflowTools failed: %v", err)
	}

	// Verify workflow-tools directory exists
	cmd := vmShell("test", "-d", "workflow-tools")
	if err := cmd.Run(); err != nil {
		t.Fatal("workflow-tools directory does not exist in VM")
	}

	// Verify expected scripts exist
	scripts := []string{"install-plugin.sh"}
	for _, script := range scripts {
		cmd = vmShell("test", "-f", "workflow-tools/"+script)
		if err := cmd.Run(); err != nil {
			t.Errorf("expected script %s not found in workflow-tools", script)
		}
	}
}

func TestInstallI2Code_Integration(t *testing.T) {
	ensureVMRunning(t)

	// Ensure workflow-tools is cloned
	cmd := vmShell("test", "-d", "workflow-tools")
	if err := cmd.Run(); err != nil {
		t.Fatal("workflow-tools not cloned, run TestCloneWorkflowTools_Integration first")
	}

	if err := InstallI2Code(vmName); err != nil {
		t.Fatalf("InstallI2Code failed: %v", err)
	}

	// Verify i2code is available
	cmd = vmShell("bash", "-lc", "which i2code")
	if err := cmd.Run(); err != nil {
		t.Error("i2code command not found after installation")
	}
}

func TestInstallUsingSDKMAN_Integration(t *testing.T) {
	ensureVMRunning(t)

	if err := InstallUsingSDKMAN(vmName); err != nil {
		t.Fatalf("InstallUsingSDKMAN failed: %v", err)
	}

	verifySDKMANToolInstalled(t, "java")
	verifySDKMANToolInstalled(t, "gradle")
}

func verifySDKMANToolInstalled(t *testing.T, tool string) {
	t.Helper()
	cmd := vmShell("bash", "-c", "source ~/.sdkman/bin/sdkman-init.sh && "+tool+" --version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s not available after SDKMAN install: %v\noutput: %s", tool, err, output)
	}
	t.Logf("%s version: %s", tool, output)
}

func TestInstallPlugins_Integration(t *testing.T) {
	ensureVMRunning(t)

	// Check that workflow-tools exists
	cmd := vmShell("test", "-d", "workflow-tools")
	if err := cmd.Run(); err != nil {
		t.Skip("workflow-tools not cloned, run TestCloneWorkflowTools_Integration first")
	}

	if err := InstallPlugins(vmName); err != nil {
		t.Fatalf("InstallPlugins failed: %v", err)
	}

	// Verify plugins are installed by checking Claude Code config
	cmd = vmShell("test", "-d", ".claude/plugins")
	if err := cmd.Run(); err != nil {
		t.Error("~/.claude/plugins directory does not exist after plugin reinstallation")
	}
}

func TestDockerRootless_RunContainer_Integration(t *testing.T) {
	ensureVMRunning(t)

	containerName := "test-docker-rootless"
	removeContainer(containerName)
	runDetachedContainer(t, containerName, "18080:80", "nginx:alpine")
	verifyContainerRunning(t, containerName)
	verifyPortAccessible(t, "18080")
	removeContainer(containerName)
}

func removeContainer(name string) {
	vmShell("bash", "-lc", "docker rm -f "+name).Run()
}

func runDetachedContainer(t *testing.T, name, portMapping, image string) {
	t.Helper()
	cmd := vmShell("bash", "-lc", "docker run -d --name "+name+" -p "+portMapping+" "+image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run failed: %v\noutput: %s", err, output)
	}
}

func verifyContainerRunning(t *testing.T, name string) {
	t.Helper()
	cmd := vmShell("bash", "-lc", "docker ps --filter name="+name+" --format '{{.Status}}'")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker ps failed: %v", err)
	}
	if !strings.Contains(string(output), "Up") {
		t.Errorf("expected container to be running, got: %s", output)
	}
}

func verifyPortAccessible(t *testing.T, port string) {
	t.Helper()
	cmd := vmShell("bash", "-lc", "curl -sf localhost:"+port)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("curl localhost:%s failed: %v", port, err)
	}
	if !strings.Contains(string(output), "<!DOCTYPE html>") {
		t.Errorf("expected HTML response, got: %s", output)
	}
}

// Task 7.1: Test ExecCommand runs commands inside VM in repo directory
func TestExecCommand_EchoHello_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	// Test echo hello
	cmdArgs := BuildExecCommand("isolarium", workdir, nil, []string{"echo", "hello"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("echo hello failed: %v", err)
	}
	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}

	// Test pwd returns repo directory
	cmdArgs = BuildExecCommand("isolarium", workdir, nil, []string{"pwd"})
	cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("pwd failed: %v", err)
	}
	if !strings.Contains(string(output), "/repo") {
		t.Errorf("expected pwd to contain '/repo', got: %s", output)
	}
}

// Task 7.2: Test ExecInteractiveCommand with TTY mode
func TestExecInteractiveCommand_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	// Use cat to echo back stdin; pipe "hello" in and capture stdout
	cmdArgs := BuildInteractiveExecCommand("isolarium", workdir, nil, []string{"cat"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = strings.NewReader("hello\n")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("interactive cat failed: %v", err)
	}
	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}
}

// Task 7.3: Test ExecCommand with environment variable injection
func TestExecCommand_WithEnvVars_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	envVars := map[string]string{"TEST_VAR": "test_value"}
	cmdArgs := BuildExecCommand("isolarium", workdir, envVars, []string{"printenv", "TEST_VAR"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("printenv TEST_VAR failed: %v", err)
	}
	if !strings.Contains(string(output), "test_value") {
		t.Errorf("expected output to contain 'test_value', got: %s", output)
	}
}

// Task 7.5: Test SIGINT terminates command in VM
func TestExecCommand_SIGINT_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	cmdArgs := BuildExecCommand("isolarium", workdir, nil, []string{"sleep", "3600"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep command: %v", err)
	}

	// Send SIGINT after 1 second
	time.AfterFunc(1*time.Second, func() {
		cmd.Process.Signal(syscall.SIGINT)
	})

	// Wait for process to terminate with a timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated as expected
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not terminate within 5 seconds after SIGINT")
	}
}

// Task 8.4: Test GetVMState returns correct state for running VM
func TestGetVMState_Integration(t *testing.T) {
	ensureVMRunning(t)

	state := GetVMState(vmName)
	if state != "running" {
		t.Errorf("expected VM state 'running', got %q", state)
	}
}

// Task 10.1: Test OpenShell opens interactive shell
func TestOpenShell_Integration(t *testing.T) {
	ensureVMRunning(t)

	cmdArgs := BuildShellCommand("isolarium", "", nil)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = strings.NewReader("echo test\nexit\n")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("shell command failed: %v", err)
	}
	if !strings.Contains(string(output), "test") {
		t.Errorf("expected output to contain 'test', got: %s", output)
	}
}

// ensureVMRunning checks if the VM exists and is running, creating or starting it if necessary.
func ensureVMRunning(t *testing.T) {
	t.Helper()

	exists, err := VMExists(vmName)
	if err != nil {
		t.Fatalf("failed to check VM status: %v", err)
	}

	if !exists {
		t.Log("VM does not exist, creating...")
		if err := CreateVM(vmName); err != nil {
			t.Fatalf("failed to create VM: %v", err)
		}
		return
	}

	state := GetVMState(vmName)
	if state == "running" {
		return
	}

	t.Log("VM is stopped, starting...")
	cmd := exec.Command("limactl", "start", vmName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}

	state = GetVMState(vmName)
	if state != "running" {
		t.Fatalf("VM is not running after start, state: %s", state)
	}
}

// vmShell creates an exec.Cmd that runs a command inside the VM via limactl shell
func vmShell(args ...string) *exec.Cmd {
	return exec.Command("limactl", append([]string{"shell", vmName, "--"}, args...)...)
}

// ensureRepoDirExists creates ~/repo in the VM if it doesn't already exist
func ensureRepoDirExists(t *testing.T) {
	t.Helper()
	if err := vmShell("mkdir", "-p", "repo").Run(); err != nil {
		t.Fatalf("failed to create ~/repo in VM: %v", err)
	}
}

// vmRepoDir returns the absolute path to ~/repo inside the VM
func vmRepoDir(t *testing.T) string {
	t.Helper()
	homeDir, err := GetVMHomeDir(vmName)
	if err != nil {
		t.Fatalf("failed to get VM home directory: %v", err)
	}
	return homeDir + "/repo"
}

func readCredentialsFromKeychain() (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-a", os.Getenv("USER"), "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
