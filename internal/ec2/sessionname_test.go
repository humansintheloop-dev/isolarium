package ec2

import (
	"errors"
	"strings"
	"testing"
)

// sessionLister stands in for `tmux list-sessions` on the instance: it records
// the command it was asked to run and answers with canned output.
type sessionLister struct {
	output    string
	exitCode  int
	err       error
	called    bool
	base      string
	publicDNS string
	command   RemoteCommand
}

func (l *sessionLister) run(base, publicDNS string, cmd RemoteCommand) (string, int, error) {
	l.called = true
	l.base = base
	l.publicDNS = publicDNS
	l.command = cmd
	return l.output, l.exitCode, l.err
}

func TestEC2NewSessionPicksLowestFreeNumber(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		want     string
	}{
		{name: "no session is running", existing: nil, want: "isolarium"},
		{name: "only the default session is running", existing: []string{"isolarium"}, want: "isolarium-2"},
		{name: "the default and the second are running", existing: []string{"isolarium", "isolarium-2"}, want: "isolarium-3"},
		{name: "a closed session left a gap", existing: []string{"isolarium", "isolarium-3"}, want: "isolarium-2"},
		{name: "an unrelated session is running", existing: []string{"scratch"}, want: "isolarium"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NextSessionName(tc.existing)

			if got != tc.want {
				t.Errorf("NextSessionName(%v) = %q, want %q", tc.existing, got, tc.want)
			}
		})
	}
}

func TestEC2NewSessionAsksTheInstanceWhichSessionsAreRunning(t *testing.T) {
	base := t.TempDir()
	lister := &sessionLister{output: "isolarium\nisolarium-3\n"}

	got, err := NewInstanceQuery(base, testPublicDNS, lister.run).ListSessions()

	if err != nil {
		t.Fatalf("ListSessions() error = %v, want nil", err)
	}
	assertCommandEquals(t, lister.command.Args, []string{"tmux", "list-sessions", "-F", "'#{session_name}'"})
	if lister.base != base {
		t.Errorf("ListSessions() used base %q, want %q", lister.base, base)
	}
	if lister.publicDNS != testPublicDNS {
		t.Errorf("ListSessions() used public DNS %q, want %q", lister.publicDNS, testPublicDNS)
	}
	assertCommandEquals(t, got, []string{"isolarium", "isolarium-3"})
}

func TestEC2NewSessionTreatsAStoppedTmuxServerAsNoSessions(t *testing.T) {
	lister := &sessionLister{exitCode: 1}

	got, err := NewInstanceQuery(t.TempDir(), testPublicDNS, lister.run).ListSessions()

	if err != nil {
		t.Fatalf("ListSessions() error = %v, want nil when no tmux server is running", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSessions() = %v, want no sessions", got)
	}
}

func TestEC2NewSessionReportsAnUnreachableInstance(t *testing.T) {
	lister := &sessionLister{err: errors.New("ssh: connect failed")}

	if _, err := NewInstanceQuery(t.TempDir(), testPublicDNS, lister.run).ListSessions(); err == nil {
		t.Fatal("ListSessions() returned nil error for an unreachable instance")
	}
}

// TestEC2NewSessionNeverKills pins the whole --new-session path down to the
// command line that reaches the instance: an additional session is started and
// nothing that could end a running one is ever sent.
func TestEC2NewSessionNeverKills(t *testing.T) {
	base := t.TempDir()
	lister := &sessionLister{output: "isolarium\n"}

	sessionName, err := NewInstanceQuery(base, testPublicDNS, lister.run).ResolveNewSessionName()
	if err != nil {
		t.Fatalf("ResolveNewSessionName() error = %v, want nil", err)
	}

	commandLine := strings.Join(BuildInteractiveExecCommand(base, testPublicDNS, RemoteCommand{
		Workdir: RemoteRepoDir,
		Args:    BuildTmuxCommand(sessionName, []string{"bash"}),
	}), " ")

	assertContainsAll(t, "the --new-session command line", commandLine, "tmux new-session -A -s isolarium-2")
	assertContainsNone(t, "the --new-session command line", commandLine, "kill-session", "kill-server")
}
