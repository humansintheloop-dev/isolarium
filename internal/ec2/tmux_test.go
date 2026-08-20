package ec2

import (
	"bytes"
	"errors"
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

func TestEC2TmuxSessionExistsAsksTheInstance(t *testing.T) {
	base := t.TempDir()
	probe := &sessionProbe{}

	SessionExists(NewInstanceSession(base, testPublicDNS, probe.run), DefaultSessionName)

	if !probe.called {
		t.Fatal("SessionExists() never reached the instance")
	}
	assertCommandEquals(t, probe.command.Args, []string{"tmux", "has-session", "-t", "isolarium"})
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
