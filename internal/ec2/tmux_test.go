package ec2

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// sessionProbe stands in for tmux on the instance: it records the command the
// existence check sends and answers with a canned exit status.
type sessionProbe struct {
	exitCode  int
	err       error
	called    bool
	base      string
	publicDNS string
	command   RemoteCommand
}

func (p *sessionProbe) run(base, publicDNS string, cmd RemoteCommand) (int, error) {
	p.called = true
	p.base = base
	p.publicDNS = publicDNS
	p.command = cmd
	return p.exitCode, p.err
}

func TestEC2TmuxCommandWrapsInteractiveCommand(t *testing.T) {
	got := BuildTmuxCommand(DefaultSessionName, []string{"claude", "-p", "hello"})

	assertCommandEquals(t, got, []string{"tmux", "new-session", "-A", "-s", "isolarium", "--", "claude", "-p", "hello"})
}

func TestEC2TmuxCommandTravelsOverTheInteractiveTransport(t *testing.T) {
	base := t.TempDir()

	got := BuildInteractiveExecCommand(base, testPublicDNS, RemoteCommand{
		Workdir: RemoteRepoDir,
		Args:    BuildTmuxCommand(DefaultSessionName, []string{"claude"}),
	})

	want := append(sshOptionSet(base),
		"-t", "ubuntu@"+testPublicDNS, "--",
		"cd", RemoteRepoDir, "&&",
		"tmux", "new-session", "-A", "-s", "isolarium", "--", "claude",
	)
	assertCommandEquals(t, got, want)
}

func TestEC2ExitStatusFilePathIsPerSession(t *testing.T) {
	tests := []struct {
		session string
		want    string
	}{
		{session: "isolarium", want: "~/.isolarium/status-isolarium"},
		{session: "isolarium-2", want: "~/.isolarium/status-isolarium-2"},
	}

	for _, tc := range tests {
		t.Run(tc.session, func(t *testing.T) {
			if got := StatusFilePath(tc.session); got != tc.want {
				t.Errorf("StatusFilePath(%q) = %q, want %q", tc.session, got, tc.want)
			}
		})
	}
}

// TestEC2ExitStatusWrapQuotesTheCommandForTheRemoteShell pins the two layers of
// quoting: the script is one single-quoted word for the remote login shell, and
// inside it the command's own arguments are quoted for the sh that runs them.
// The status is recorded the moment the command ends; the pane is then held
// open for a second, because tmux tears the window down as soon as its process
// exits and a command that ends within milliseconds would otherwise be gone
// before the client that started it has drawn the pane — so `echo ok` would
// stream nothing at all.
func TestEC2ExitStatusWrapQuotesTheCommandForTheRemoteShell(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "plain words",
			args: []string{"claude", "-p", "hello"},
			want: `'claude -p hello; echo $? > ~/.isolarium/status-isolarium; sleep 1'`,
		},
		{
			name: "an argument with a space",
			args: []string{"sh", "-c", "exit 3"},
			want: `'sh -c '\''exit 3'\''; echo $? > ~/.isolarium/status-isolarium; sleep 1'`,
		},
		{
			name: "an argument with a quote",
			args: []string{"echo", "it's"},
			want: `'echo '\''it'\''\'\'''\''s'\''; echo $? > ~/.isolarium/status-isolarium; sleep 1'`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := WrapWithExitStatus(tc.args, StatusFilePath(DefaultSessionName))

			assertCommandEquals(t, got, []string{"sh", "-c", tc.want})
		})
	}
}

func TestEC2TmuxDetachableCommandStartsANamedSessionAndRecordsTheCommand(t *testing.T) {
	got := BuildDetachableTmuxCommand(DefaultSessionName, []string{"claude", "-p", "hello"})

	assertCommandEquals(t, got, []string{
		"tmux", "new-session", "-s", "isolarium", "--",
		"sh", "-c", `'claude -p hello; echo $? > ~/.isolarium/status-isolarium; sleep 1'`,
		`\;`, "set-option", "-t", "isolarium", "@isolarium-command", "'claude -p hello'",
	})
}

func TestEC2TmuxDetachableCommandNeverAttachesToARunningSession(t *testing.T) {
	got := strings.Join(BuildDetachableTmuxCommand(DefaultSessionName, []string{"claude"}), " ")

	assertContainsNone(t, "the detachable tmux command", got, " -A ", "attach-session")
}

func TestEC2TmuxDetachableCommandTravelsOverTheSessionTransport(t *testing.T) {
	base := t.TempDir()

	got := strings.Join(BuildSessionExecCommand(base, testPublicDNS, RemoteCommand{
		Workdir: RemoteRepoDir,
		Args:    BuildDetachableTmuxCommand(DefaultSessionName, []string{"claude"}),
	}), " ")

	assertContainsAll(t, "the session command line", got,
		"-tt ubuntu@"+testPublicDNS,
		"tmux new-session -s isolarium -- sh -c 'claude; echo $? > ~/.isolarium/status-isolarium; sleep 1'",
		"@isolarium-command 'claude'",
	)
}

func TestEC2TmuxCommandRecordQuotesOnlyWhatTheShellWouldSplit(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "plain words", args: []string{"claude", "-p", "hello"}, want: "claude -p hello"},
		{name: "an argument with a space", args: []string{"sh", "-c", "exit 3"}, want: "sh -c 'exit 3'"},
		{name: "an argument with a quote", args: []string{"echo", "it's"}, want: `echo 'it'\''s'`},
		{name: "paths and flags", args: []string{"i2code", "--isolated", "docs/ideas/x", "--with-sdkman"}, want: "i2code --isolated docs/ideas/x --with-sdkman"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CommandRecord(tc.args); got != tc.want {
				t.Errorf("CommandRecord(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestEC2TmuxAttachCommandJoinsTheNamedSession(t *testing.T) {
	got := BuildAttachCommand("isolarium-2")

	assertCommandEquals(t, got, []string{"tmux", "attach-session", "-t", "isolarium-2"})
}

func TestEC2TmuxRecordedCommandAsksTheInstance(t *testing.T) {
	base := t.TempDir()
	lister := &sessionLister{output: "claude -p hello\n"}

	got, err := NewInstanceQuery(base, testPublicDNS, lister.run).RecordedCommand(DefaultSessionName)

	if err != nil {
		t.Fatalf("RecordedCommand() error = %v, want nil", err)
	}
	assertCommandEquals(t, lister.command.Args, []string{"tmux", "show-option", "-qv", "-t", "isolarium", "@isolarium-command"})
	if lister.base != base {
		t.Errorf("RecordedCommand() used base %q, want %q", lister.base, base)
	}
	if lister.publicDNS != testPublicDNS {
		t.Errorf("RecordedCommand() used public DNS %q, want %q", lister.publicDNS, testPublicDNS)
	}
	if got != "claude -p hello" {
		t.Errorf("RecordedCommand() = %q, want %q", got, "claude -p hello")
	}
}

func TestEC2TmuxRecordedCommandIsEmptyWhenTheSessionRecordedNone(t *testing.T) {
	tests := []struct {
		name   string
		lister sessionLister
	}{
		{name: "the option is unset", lister: sessionLister{output: "\n"}},
		{name: "tmux rejects the query", lister: sessionLister{exitCode: 1, output: "no such session"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lister := tc.lister

			got, err := NewInstanceQuery(t.TempDir(), testPublicDNS, lister.run).RecordedCommand(DefaultSessionName)

			if err != nil {
				t.Fatalf("RecordedCommand() error = %v, want nil", err)
			}
			if got != "" {
				t.Errorf("RecordedCommand() = %q, want no recorded command", got)
			}
		})
	}
}

func TestEC2TmuxRecordedCommandReportsAnUnreachableInstance(t *testing.T) {
	lister := &sessionLister{err: errors.New("ssh: connect failed")}

	if _, err := NewInstanceQuery(t.TempDir(), testPublicDNS, lister.run).RecordedCommand(DefaultSessionName); err == nil {
		t.Fatal("RecordedCommand() returned nil error for an unreachable instance")
	}
}

func TestEC2TmuxSessionExistsAsksTheInstance(t *testing.T) {
	base := t.TempDir()
	probe := &sessionProbe{}

	SessionExists(NewInstanceSession(base, testPublicDNS, probe.run), DefaultSessionName)

	if !probe.called {
		t.Fatal("SessionExists() never reached the instance")
	}
	assertCommandEquals(t, probe.command.Args, []string{"tmux", "has-session", "-t", "isolarium", "2>/dev/null"})
	if probe.base != base {
		t.Errorf("SessionExists() used base %q, want %q", probe.base, base)
	}
	if probe.publicDNS != testPublicDNS {
		t.Errorf("SessionExists() used public DNS %q, want %q", probe.publicDNS, testPublicDNS)
	}
}

func TestEC2TmuxSessionExistsFollowsTheRemoteExitStatus(t *testing.T) {
	tests := []struct {
		name  string
		probe sessionProbe
		want  bool
	}{
		{name: "has-session succeeds", probe: sessionProbe{exitCode: 0}, want: true},
		{name: "has-session fails", probe: sessionProbe{exitCode: 1}, want: false},
		{name: "the instance is unreachable", probe: sessionProbe{err: errors.New("ssh: connect failed")}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := tc.probe

			got := SessionExists(NewInstanceSession(t.TempDir(), testPublicDNS, probe.run), DefaultSessionName)

			if got != tc.want {
				t.Errorf("SessionExists() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEC2TmuxNoticePrintedOnlyWhenSessionExists(t *testing.T) {
	tests := []struct {
		name  string
		probe sessionProbe
		want  string
	}{
		{
			name:  "a session is already running",
			probe: sessionProbe{exitCode: 0},
			want:  "attaching to existing session 'isolarium'; use --new-session to start a fresh one\n",
		},
		{name: "no session is running", probe: sessionProbe{exitCode: 1}, want: ""},
		{name: "the instance is unreachable", probe: sessionProbe{err: errors.New("ssh: connect failed")}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := tc.probe
			var notice bytes.Buffer

			AnnounceReattach(&notice, NewInstanceSession(t.TempDir(), testPublicDNS, probe.run), DefaultSessionName)

			if notice.String() != tc.want {
				t.Errorf("notice = %q, want %q", notice.String(), tc.want)
			}
		})
	}
}
