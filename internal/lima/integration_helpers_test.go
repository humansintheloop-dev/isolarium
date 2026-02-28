//go:build integration || integration_setup || integration_teardown

package lima

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/github"
)

var parseRepoURL = github.ParseRepoURL
var newTokenMinter = github.NewTokenMinter

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
		setEnvFromLine(scanner.Text())
	}
}

func setEnvFromLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

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

func vmShell(args ...string) *exec.Cmd {
	return exec.Command("limactl", append([]string{"shell", vmName, "--"}, args...)...)
}

func ensureRepoDirExists(t *testing.T) {
	t.Helper()
	if err := vmShell("mkdir", "-p", "repo").Run(); err != nil {
		t.Fatalf("failed to create ~/repo in VM: %v", err)
	}
}

func vmRepoDir(t *testing.T) string {
	t.Helper()
	homeDir, err := GetVMHomeDir(vmName)
	if err != nil {
		t.Fatalf("failed to get VM home directory: %v", err)
	}
	return homeDir + "/repo"
}

func verifyToolInPath(t *testing.T, tool string) {
	t.Helper()
	cmd := vmShell("which", tool)
	if err := cmd.Run(); err != nil {
		t.Errorf("tool %s not found in VM PATH", tool)
	}
}

func verifyJavaHomeSet(t *testing.T) {
	t.Helper()
	cmd := vmShell("grep", "JAVA_HOME", "/etc/environment")
	if err := cmd.Run(); err != nil {
		t.Error("JAVA_HOME not set in /etc/environment")
	}
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

func requireWorkflowToolsCloned(t *testing.T) {
	t.Helper()
	cmd := vmShell("test", "-d", "workflow-tools")
	if err := cmd.Run(); err != nil {
		t.Fatal("workflow-tools not cloned, run TestCloneWorkflowTools_Integration first")
	}
}

func hostGitBranch(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func hostGitRemoteURL(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get remote URL: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func mintGitHubToken(t *testing.T, remoteURL string) string {
	t.Helper()
	loadTestEnvFile(t)

	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if appID == "" || privateKeyPath == "" {
		t.Fatal("GitHub App credentials not configured - set GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY_PATH in .env.local")
	}

	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		t.Fatalf("failed to read private key: %v", err)
	}

	owner, repo, err := parseRepoURL(remoteURL)
	if err != nil {
		t.Fatalf("failed to parse repo URL: %v", err)
	}

	minter, err := newTokenMinter(appID, string(privateKeyBytes), "")
	if err != nil {
		t.Fatalf("failed to create token minter: %v", err)
	}

	token, err := minter.MintInstallationToken(owner, repo)
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}
	t.Log("Token minted successfully")
	return token
}

func readCredentialsFromFile(t *testing.T) string {
	t.Helper()
	loadTestEnvFile(t)
	credentialsPath := os.Getenv("CLAUDE_CREDENTIALS_PATH")
	if credentialsPath == "" {
		t.Fatal("no credentials available: Keychain failed and CLAUDE_CREDENTIALS_PATH not set")
	}
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatalf("failed to read credentials file %s: %v", credentialsPath, err)
	}
	return string(data)
}

func verifyVMFileExists(t *testing.T, path string) {
	t.Helper()
	cmd := vmShell("test", "-f", path)
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected file %s does not exist in VM", path)
	}
}

func verifyVMDirExists(t *testing.T, path string) {
	t.Helper()
	cmd := vmShell("test", "-d", path)
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected directory %s does not exist in VM", path)
	}
}
