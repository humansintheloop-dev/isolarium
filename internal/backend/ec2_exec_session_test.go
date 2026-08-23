package backend

import (
	"bytes"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// sessionStart is the command line Exec sends when no session is running: a
// detachable session running the command under sh, which writes the command's
// exit status to the session's status file when it ends and then holds the pane
// open for a second so the client has drawn it, with the record of the
// unwrapped command set on the session in the same tmux invocation. Both the
// script and the record travel through the remote shell in single quotes, with
// any single quote inside them closed, escaped, and reopened.
func sessionStart(session, record string) []string {
	script := singleQuoted(record + "; echo $? > ~/.isolarium/status-" + session + "; sleep 1")
	start := []string{"tmux", "new-session", "-s", session, "--", "sh", "-c", script}
	return append(start, `\;`, "set-option", "-t", session, "@isolarium-command", singleQuoted(record))
}

func singleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func TestEC2TmuxBackendRunsExecInsideADetachableSession(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)

	if _, err := f.backend.Exec(ExecRequest{
		ContainerName: "my-work",
		EnvVars:       map[string]string{"GH_TOKEN": "tok123"},
		Args:          []string{"echo", "hello"},
	}); err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}

	if !f.session.called {
		t.Fatal("Exec() did not reach the session SSH transport")
	}
	if f.session.publicDNS != ec2SpyPublicDNS {
		t.Errorf("Exec() used public DNS %q, want %q", f.session.publicDNS, ec2SpyPublicDNS)
	}
	if f.session.command.Workdir != ec2.RemoteRepoDir {
		t.Errorf("Exec() workdir = %q, want %q", f.session.command.Workdir, ec2.RemoteRepoDir)
	}
	if f.session.command.EnvVars["GH_TOKEN"] != "tok123" {
		t.Errorf("Exec() passed GH_TOKEN %q, want %q", f.session.command.EnvVars["GH_TOKEN"], "tok123")
	}
	assertArgsEqual(t, "exec args", f.session.command.Args, sessionStart("isolarium", "echo hello"))
	assertSessionProbe(t, f.exec)
	if f.interactive.called {
		t.Error("Exec() reached the interactive transport, want the session transport alone")
	}
}

func TestEC2TmuxBackendExecReattachesWhenTheSameCommandIsRunning(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 4)
	f.exec.exitCode = 0
	f.instance.recorded = "claude -p hello"
	var notice bytes.Buffer
	f.backend.ErrWriter = &notice

	exitCode, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"claude", "-p", "hello"}})

	if err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}
	if exitCode != 4 {
		t.Errorf("Exec() exit code = %d, want the 4 the command recorded", exitCode)
	}
	if !f.instance.asked("tmux show-option -qv -t isolarium @isolarium-command") {
		t.Error("Exec() never read the running session's recorded command")
	}
	assertArgsEqual(t, "exec args", f.session.command.Args, []string{"tmux", "attach-session", "-t", "isolarium"})
	if strings.Contains(strings.Join(f.session.command.Args, " "), "new-session") {
		t.Errorf("Exec() started a second session: %v", f.session.command.Args)
	}
	if want := "reattaching to session 'isolarium', which is already running: claude -p hello\n"; notice.String() != want {
		t.Errorf("Exec() wrote %q to stderr, want %q", notice.String(), want)
	}
}

func TestEC2TmuxBackendExecRefusesASessionRunningSomethingElse(t *testing.T) {
	tests := []struct {
		name        string
		recorded    string
		wantNamed   []string
		wantAbsent  string
		description string
	}{
		{
			name:      "a different command",
			recorded:  "claude -p other",
			wantNamed: []string{"'echo hello'", "'claude -p other'", "session 'isolarium'", "isolarium shell --type ec2", "--new-session"},
		},
		{
			name:       "a session that recorded no command",
			recorded:   "",
			wantNamed:  []string{"'echo hello'", "isolarium run did not start", "isolarium shell --type ec2"},
			wantAbsent: "running ''",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := ec2BackendWithRecordedInstance(t, 0)
			f.instance.recorded = tc.recorded

			exitCode, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"echo", "hello"}})

			if err == nil {
				t.Fatal("Exec() returned nil error while the session runs something else")
			}
			if exitCode != 1 {
				t.Errorf("Exec() exit code = %d, want 1", exitCode)
			}
			if f.session.called {
				t.Errorf("Exec() reached the session transport with %v, want nothing run", f.session.command.Args)
			}
			assertContainsAll(t, "the refusal", err.Error(), tc.wantNamed...)
			if tc.wantAbsent != "" {
				assertContainsNone(t, "the refusal", err.Error(), tc.wantAbsent)
			}
		})
	}
}

func TestEC2NewSessionBackendRunsExecInAnAdditionalSession(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)
	f.instance.sessions = "isolarium\n"
	f.backend.UseNewSession()

	if _, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"bash"}}); err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}

	if !f.instance.asked("tmux list-sessions") {
		t.Fatal("--new-session never asked the instance which sessions are running")
	}
	assertArgsEqual(t, "exec args", f.session.command.Args, sessionStart("isolarium-2", "bash"))
	assertArgsEqual(t, "session probe", f.exec.command.Args, []string{"tmux", "has-session", "-t", "isolarium-2", "2>/dev/null"})
}
