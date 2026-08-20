//go:build ec2

package ec2_test

import (
	"strings"
	"testing"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const userDataLimit = 16384

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
	environment.assertUnprivilegedUserNamespacesAreUnrestricted()
}

// toolchainProbe is one tool the instance is expected to carry, together with
// the command that proves it is installed and runnable.
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
		{"docker", []string{"docker", "info"}},
	}
}

func (e *ec2Environment) assertCloudInitReportsDone() {
	e.t.Helper()

	exitCode, output := e.run("cloud-init", "status", "--wait")
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

	exitCode, output := e.run(probe.args...)
	if exitCode != 0 {
		e.t.Errorf("%s: %s exited %d, want 0", probe.tool, strings.Join(probe.args, " "), exitCode)
		return
	}
	e.t.Logf("%s: %s", probe.tool, firstLine(output))
}

func (e *ec2Environment) assertUnprivilegedUserNamespacesAreUnrestricted() {
	e.t.Helper()

	exitCode, output := e.run("sysctl", "kernel.apparmor_restrict_unprivileged_userns")
	if exitCode != 0 {
		e.t.Fatalf("sysctl kernel.apparmor_restrict_unprivileged_userns exited %d, want 0", exitCode)
	}
	want := "kernel.apparmor_restrict_unprivileged_userns = 0"
	if strings.TrimSpace(output) != want {
		e.t.Errorf("sysctl reported %q, want %q — rootless Docker cannot start while user namespaces are restricted",
			strings.TrimSpace(output), want)
	}
}

// run executes args on the instance exactly as isolarium run does, so a tool
// that is only reachable from an interactive login shell counts as missing.
func (e *ec2Environment) run(args ...string) (int, string) {
	e.t.Helper()

	var exitCode int
	var err error
	output := captureStdout(e.t, func() {
		exitCode, err = e.backend.Exec(backend.ExecRequest{ContainerName: e.name, Args: args})
	})
	if err != nil {
		e.t.Fatalf("Exec of %q: %v", strings.Join(args, " "), err)
	}
	return exitCode, output
}

func firstLine(output string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	return line
}
