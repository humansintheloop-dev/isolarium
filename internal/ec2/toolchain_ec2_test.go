//go:build ec2

package ec2_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	userDataLimit    = 16384
	claudeBinaryPath = "/home/ubuntu/.local/bin/claude"
	claudeOwner      = "ubuntu"
	npmClaudeCode    = "@anthropic-ai/claude-code"
	i2codeBinaryPath = "/home/ubuntu/.local/bin/i2code"
	sdkmanConfigPath = "/home/ubuntu/.sdkman/etc/config"
	javaLinkPath     = "/usr/local/bin/java"
)

// TestEC2Instance_HasToolchain proves that the cloud-init document carried as
// user_data actually provisions a usable instance, rather than merely rendering
// the right text host-side.
func TestEC2Instance_HasToolchain(t *testing.T) {
	reportRenderedUserDataSize(t)
	environment := sharedInstance(t)

	environment.assertCloudInitReportsDone()
	for _, probe := range toolchainProbes() {
		environment.assertToolIsInstalled(probe)
	}
	environment.assertClaudeIsInstalledForTheUserThatRunsIt()
	environment.assertWorkflowToolsIsCloned()
	environment.assertUnprivilegedUserNamespacesAreUnrestricted()
	environment.reportSDKMANInstallDuration()
	environment.reportWorkflowToolsInstallDuration()
}

// The SDKMAN script rewrites ~/.sdkman/etc/config as its first step and links
// /usr/local/bin/java as its last, so the two mtimes bound how long the
// post-cloud-init install added to create.
func (e *ec2Environment) reportSDKMANInstallDuration() {
	e.t.Helper()

	started := e.modificationTime(sdkmanConfigPath)
	finished := e.modificationTime(javaLinkPath)
	e.t.Logf("TIMING: SDKMAN install of Java and Gradle took %s", finished.Sub(started).Round(time.Second))
}

// The workflow-tools clone starts as soon as the SDKMAN script has linked java,
// and `uv tool install` writes the i2code launcher as its last step, so the two
// mtimes bound what cloning and installing i2code added to create.
func (e *ec2Environment) reportWorkflowToolsInstallDuration() {
	e.t.Helper()

	started := e.modificationTime(javaLinkPath)
	finished := e.modificationTime(i2codeBinaryPath)
	e.t.Logf("TIMING: workflow-tools clone and i2code install took %s", finished.Sub(started).Round(time.Second))
}

// `i2code --help` alone would also pass with a launcher installed from
// somewhere else, so the clone it was installed from is asserted separately, as
// the Lima integration test does before it installs.
func (e *ec2Environment) assertWorkflowToolsIsCloned() {
	e.t.Helper()

	exitCode, output := e.askInstance("test", "-d", ec2.RemoteWorkflowToolsDir)
	if exitCode != 0 {
		e.t.Errorf("test -d %s exited %d, want 0; output: %s", ec2.RemoteWorkflowToolsDir, exitCode, output)
	}
}

func (e *ec2Environment) modificationTime(path string) time.Time {
	e.t.Helper()

	exitCode, output := e.askInstance("stat", "-c", "%Y", path)
	if exitCode != 0 {
		e.t.Fatalf("stat -c %%Y %s exited %d, want 0; output: %s", path, exitCode, output)
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		e.t.Fatalf("stat -c %%Y %s printed %q, want epoch seconds", path, output)
	}
	return time.Unix(seconds, 0)
}

// The root-owned npm global install this replaced answered `claude --version`
// just as well, so proving the switch means saying where the binary resolves and
// who owns it: only an install the ubuntu user made can rewrite itself when
// Claude Code auto-updates.
func (e *ec2Environment) assertClaudeIsInstalledForTheUserThatRunsIt() {
	e.t.Helper()

	e.assertCommandPrints(claudeBinaryPath, "command", "-v", "claude")
	e.assertCommandPrints(claudeOwner, "stat", "-c", "%U", claudeBinaryPath)
	e.assertNpmGlobalTreeHasNoClaudeCode()
}

func (e *ec2Environment) assertCommandPrints(want string, args ...string) {
	e.t.Helper()

	command := strings.Join(args, " ")
	exitCode, output := e.askInstance(args...)
	if exitCode != 0 {
		e.t.Fatalf("%s exited %d, want 0; output: %s", command, exitCode, output)
	}
	if got := strings.TrimSpace(output); got != want {
		e.t.Errorf("%s printed %q, want %q", command, got, want)
	}
}

func (e *ec2Environment) assertNpmGlobalTreeHasNoClaudeCode() {
	e.t.Helper()

	exitCode, output := e.askInstance("npm", "ls", "-g", "--depth=0")
	if exitCode != 0 {
		e.t.Fatalf("npm ls -g --depth=0 exited %d, want 0; output: %s", exitCode, output)
	}
	if strings.Contains(output, npmClaudeCode) {
		e.t.Errorf("the root-owned npm global tree still carries %s: %s", npmClaudeCode, strings.TrimSpace(output))
	}
}

// toolchainProbe is one tool the instance is expected to carry, together with
// the command that proves it is installed and runnable. i2code is probed with
// --help because its CLI has no --version option.
type toolchainProbe struct {
	tool string
	args []string
}

func toolchainProbes() []toolchainProbe {
	return []toolchainProbe{
		{"git", []string{"git", "--version"}},
		{"gh", []string{"gh", "--version"}},
		{"node", []string{"node", "--version"}},
		{"tmux", []string{"tmux", "-V"}},
		{"uv", []string{"uv", "--version"}},
		{"claude", []string{"claude", "--version"}},
		{"i2code", []string{"i2code", "--help"}},
		{"docker", []string{"docker", "info"}},
		{"java", []string{"java", "-version"}},
		{"gradle", []string{"bash", "-lc", "'source ~/.sdkman/bin/sdkman-init.sh && gradle --version'"}},
	}
}

func (e *ec2Environment) assertCloudInitReportsDone() {
	e.t.Helper()

	exitCode, output := e.askInstance("cloud-init", "status", "--wait")
	if exitCode != 0 {
		e.t.Fatalf("cloud-init status --wait exited %d, want 0; output: %s", exitCode, output)
	}
	if !strings.Contains(output, "status: done") {
		e.t.Errorf("cloud-init status --wait reported %q, want it to report %q", strings.TrimSpace(output), "status: done")
	}
	e.t.Logf("TIMING: cloud-init reported done %s after create started", time.Since(e.createdAt).Round(time.Second))
}

// reportRenderedUserDataSize records how much of the 16 KB user_data budget the
// cloud-init document that provisioned this instance actually consumed.
func reportRenderedUserDataSize(t *testing.T) {
	t.Logf("SIZE: rendered user_data is %d bytes of the %d-byte limit", len(ec2.RenderUserData()), userDataLimit)
}

func (e *ec2Environment) assertToolIsInstalled(probe toolchainProbe) {
	e.t.Helper()

	exitCode, output := e.askInstance(probe.args...)
	if exitCode != 0 {
		e.t.Errorf("%s: %s exited %d, want 0", probe.tool, strings.Join(probe.args, " "), exitCode)
		return
	}
	e.t.Logf("%s: %s", probe.tool, firstLine(output))
}

func (e *ec2Environment) assertUnprivilegedUserNamespacesAreUnrestricted() {
	e.t.Helper()

	exitCode, output := e.askInstance("sysctl", "kernel.apparmor_restrict_unprivileged_userns")
	if exitCode != 0 {
		e.t.Fatalf("sysctl kernel.apparmor_restrict_unprivileged_userns exited %d, want 0", exitCode)
	}
	want := "kernel.apparmor_restrict_unprivileged_userns = 0"
	if strings.TrimSpace(output) != want {
		e.t.Errorf("sysctl reported %q, want %q — rootless Docker cannot start while user namespaces are restricted",
			strings.TrimSpace(output), want)
	}
}

func firstLine(output string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	return line
}
