package backend

import (
	"bytes"
	"os"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const reattachNotice = "attaching to existing session 'isolarium'; use --new-session to start a fresh one\n"

func tmuxWrapped(args ...string) []string {
	return append([]string{"tmux", "new-session", "-A", "-s", "isolarium", "--"}, args...)
}

func TestEC2TmuxBackendWrapsTheInteractiveCommand(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)

	_, err := f.backend.ExecInteractive(ExecRequest{
		ContainerName: "my-work",
		EnvVars:       map[string]string{"GH_TOKEN": "tok123"},
		Args:          []string{"claude"},
	})

	if err != nil {
		t.Fatalf("ExecInteractive() error = %v, want nil", err)
	}
	if !f.interactive.called {
		t.Fatal("ExecInteractive() did not reach the interactive SSH transport")
	}
	if f.interactive.command.Workdir != ec2.RemoteRepoDir {
		t.Errorf("ExecInteractive() workdir = %q, want %q", f.interactive.command.Workdir, ec2.RemoteRepoDir)
	}
	if f.interactive.command.EnvVars["GH_TOKEN"] != "tok123" {
		t.Errorf("ExecInteractive() passed GH_TOKEN %q, want %q", f.interactive.command.EnvVars["GH_TOKEN"], "tok123")
	}
	assertArgsEqual(t, "interactive args", f.interactive.command.Args, tmuxWrapped("claude"))
}

func TestEC2TmuxBackendOpensALoginShellInTheRepository(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 5)

	exitCode, err := f.backend.OpenShell(ExecRequest{
		ContainerName: "my-work",
		EnvVars:       map[string]string{"FOO": "bar"},
	})

	if err != nil {
		t.Fatalf("OpenShell() error = %v, want nil", err)
	}
	if exitCode != 5 {
		t.Errorf("OpenShell() exit code = %d, want 5", exitCode)
	}
	if !f.interactive.called {
		t.Fatal("OpenShell() did not reach the interactive SSH transport")
	}
	if f.interactive.publicDNS != ec2SpyPublicDNS {
		t.Errorf("OpenShell() used public DNS %q, want %q", f.interactive.publicDNS, ec2SpyPublicDNS)
	}
	if f.interactive.command.Workdir != ec2.RemoteRepoDir {
		t.Errorf("OpenShell() workdir = %q, want %q", f.interactive.command.Workdir, ec2.RemoteRepoDir)
	}
	if f.interactive.command.EnvVars["FOO"] != "bar" {
		t.Errorf("OpenShell() passed FOO %q, want %q", f.interactive.command.EnvVars["FOO"], "bar")
	}
	assertArgsEqual(t, "shell args", f.interactive.command.Args, tmuxWrapped("bash", "-il"))
}

func TestEC2TmuxBackendFailsToOpenAShellOnAnEnvironmentThatWasNeverCreated(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)

	exitCode, err := f.backend.OpenShell(ExecRequest{ContainerName: "never-created"})

	if err == nil {
		t.Fatal("OpenShell() returned nil error for an environment with no metadata.json")
	}
	if exitCode != 1 {
		t.Errorf("OpenShell() exit code = %d, want 1", exitCode)
	}
	if f.interactive.called {
		t.Error("OpenShell() reached the SSH transport despite the missing metadata")
	}
}

func TestEC2TmuxBackendLeavesExecOutsideTmux(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)

	if _, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"echo", "hello"}}); err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}

	assertArgsEqual(t, "exec args", f.exec.command.Args, []string{"echo", "hello"})
}

func TestEC2TmuxNoticePrintedOnlyWhenSessionExists(t *testing.T) {
	entryPoints := []struct {
		name string
		run  func(*EC2Backend) (int, error)
	}{
		{
			name: "ExecInteractive",
			run: func(b *EC2Backend) (int, error) {
				return b.ExecInteractive(ExecRequest{ContainerName: "my-work", Args: []string{"claude"}})
			},
		},
		{
			name: "OpenShell",
			run: func(b *EC2Backend) (int, error) {
				return b.OpenShell(ExecRequest{ContainerName: "my-work"})
			},
		},
	}
	sessions := []struct {
		name      string
		probeExit int
		want      string
	}{
		{name: "a session is already running", probeExit: 0, want: reattachNotice},
		{name: "no session is running", probeExit: 1, want: ""},
	}

	for _, entry := range entryPoints {
		for _, session := range sessions {
			t.Run(entry.name+"/"+session.name, func(t *testing.T) {
				f := ec2BackendWithRecordedInstance(t, 0)
				f.exec.exitCode = session.probeExit
				var notice bytes.Buffer
				f.backend.ErrWriter = &notice

				if _, err := entry.run(f.backend); err != nil {
					t.Fatalf("%s() error = %v, want nil", entry.name, err)
				}

				if notice.String() != session.want {
					t.Errorf("%s() wrote %q to stderr, want %q", entry.name, notice.String(), session.want)
				}
				assertSessionProbe(t, f.exec)
			})
		}
	}
}

func assertSessionProbe(t *testing.T, spy *ec2ExecSpy) {
	t.Helper()

	if !spy.called {
		t.Fatal("the tmux session was never probed for")
	}
	assertArgsEqual(t, "session probe", spy.command.Args, []string{"tmux", "has-session", "-t", "isolarium"})
	if spy.command.Workdir != "" {
		t.Errorf("session probe workdir = %q, want it to run from the login directory", spy.command.Workdir)
	}
}

func TestEC2TmuxNoticeGoesToStderrByDefault(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	f.backend.ErrWriter = nil

	if f.backend.errOut() != os.Stderr {
		t.Error("the reattach notice does not default to stderr, want it kept out of the command's own output")
	}
}
