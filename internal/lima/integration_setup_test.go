//go:build integration_setup

package lima

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/claude"
)

func TestVMHasRequiredTools_Integration(t *testing.T) {
	ensureVMRunning(t)

	tools := []string{"git", "node", "docker", "gh", "claude"}
	for _, tool := range tools {
		cmd := vmShell("which", tool)
		if err := cmd.Run(); err != nil {
			t.Errorf("tool %s not found in VM", tool)
		}
	}
}

func TestInstallUsingSDKMAN_Integration(t *testing.T) {
	ensureVMRunning(t)

	if err := InstallUsingSDKMAN(vmName); err != nil {
		t.Fatalf("InstallUsingSDKMAN failed: %v", err)
	}

	verifySDKMANToolInstalled(t, "java")
	verifySDKMANToolInstalled(t, "gradle")

	verifyToolInPath(t, "java")
	verifyToolInPath(t, "javac")
	verifyJavaHomeSet(t)
}

func TestCloneRepoWithToken_Integration(t *testing.T) {
	ensureVMRunning(t)

	expectedBranch := hostGitBranch(t)
	remoteURL := hostGitRemoteURL(t)
	t.Logf("Testing with repo: %s, branch: %s", remoteURL, expectedBranch)

	vmShell("rm", "-rf", "repo").Run()

	token := mintGitHubToken(t, remoteURL)

	if err := CloneRepo(vmName, findProjectRoot(t), remoteURL, expectedBranch, token); err != nil {
		t.Fatalf("CloneRepo failed: %v", err)
	}

	verifyVMFileExists(t, "repo/go.mod")
	verifyClonedBranch(t, expectedBranch)
}

func verifyClonedBranch(t *testing.T, expectedBranch string) {
	t.Helper()
	cmd := vmShell("git", "-C", "repo", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch in VM: %v", err)
	}
	actualBranch := strings.TrimSpace(string(output))
	if actualBranch != expectedBranch {
		t.Errorf("expected branch %q, got %q", expectedBranch, actualBranch)
	}
}

func TestCloneWorkflowTools_Integration(t *testing.T) {
	ensureVMRunning(t)

	vmShell("rm", "-rf", "workflow-tools").Run()

	if err := CloneWorkflowTools(vmName, ""); err != nil {
		t.Fatalf("CloneWorkflowTools failed: %v", err)
	}

	verifyVMDirExists(t, "workflow-tools")

	scripts := []string{"install-plugin.sh"}
	for _, script := range scripts {
		verifyVMFileExists(t, "workflow-tools/"+script)
	}
}

func TestInstallI2Code_Integration(t *testing.T) {
	ensureVMRunning(t)
	requireWorkflowToolsCloned(t)

	if err := InstallI2Code(vmName); err != nil {
		t.Fatalf("InstallI2Code failed: %v", err)
	}

	cmd := vmShell("bash", "-lc", "which i2code")
	if err := cmd.Run(); err != nil {
		t.Error("i2code command not found after installation")
	}
}

func TestInstallPlugins_Integration(t *testing.T) {
	ensureVMRunning(t)
	requireWorkflowToolsCloned(t)

	if err := InstallPlugins(vmName); err != nil {
		t.Fatalf("InstallPlugins failed: %v", err)
	}

	verifyVMDirExists(t, ".claude/plugins")
}

func TestCopyClaudeCredentials_Integration(t *testing.T) {
	credentials, err := claude.ReadCredentialsFromKeychain()
	if err != nil {
		t.Logf("Keychain read failed (%v), falling back to CLAUDE_CREDENTIALS_PATH", err)
		credentials = readCredentialsFromFile(t)
	}

	ensureVMRunning(t)
	vmShell("bash", "-c", "rm -rf ~/.claude").Run()

	if err := CopyClaudeCredentials(vmName, credentials); err != nil {
		t.Fatalf("CopyClaudeCredentials failed: %v", err)
	}

	verifyVMFileExists(t, ".claude/.credentials.json")
	verifyCredentialsPermissions(t)
	verifyClaudeCanCreateFile(t)
}

func verifyCredentialsPermissions(t *testing.T) {
	t.Helper()
	cmd := vmShell("stat", "-c", "%a", ".claude/.credentials.json")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to check file permissions: %v", err)
	}
	perms := strings.TrimSpace(string(output))
	if perms != "600" {
		t.Errorf("expected permissions 600, got %s", perms)
	}
}

func verifyClaudeCanCreateFile(t *testing.T) {
	t.Helper()
	cmd := vmShell("bash", "-c",
		"cd /tmp && claude --allowed-tools Write -p 'Create a Java hello world called HelloWorld.java'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("claude command failed: %v\noutput: %s", err, output)
	}
	t.Logf("Claude response: %s", output)

	verifyVMFileExists(t, "/tmp/HelloWorld.java")

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
