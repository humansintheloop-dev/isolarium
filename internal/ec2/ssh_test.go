package ec2

import (
	"reflect"
	"strings"
	"testing"
)

const testPublicDNS = "ec2-203-0-113-7.compute-1.amazonaws.com"

func sshOptionSet(base string) []string {
	return []string{
		"ssh",
		"-i", PrivateKeyPath(base),
		"-o", "UserKnownHostsFile=" + KnownHostsPath(base),
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "IdentitiesOnly=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=4",
	}
}

func assertCommandEquals(t *testing.T, got, want []string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("command =\n  %s\nwant\n  %s", strings.Join(got, " "), strings.Join(want, " "))
	}
}

func TestBuildSSHArgsDiffersOnlyInTerminalHandling(t *testing.T) {
	tests := []struct {
		name     string
		tty      ttyMode
		ttyFlags []string
	}{
		{name: "no terminal", tty: noTTY, ttyFlags: nil},
		{name: "a terminal when the host has one", tty: requestTTY, ttyFlags: []string{"-t"}},
		{name: "a terminal even without one on the host", tty: forceTTY, ttyFlags: []string{"-tt"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()

			got := buildSSHArgs(base, testPublicDNS, tc.tty)

			want := append(sshOptionSet(base), tc.ttyFlags...)
			assertCommandEquals(t, got, append(want, "ubuntu@"+testPublicDNS))
		})
	}
}

func TestEC2ExecCommandArgs(t *testing.T) {
	base := t.TempDir()
	envVars := map[string]string{"GH_TOKEN": "tok123", "GIT_TOKEN": "tok123"}

	got := BuildExecCommand(base, testPublicDNS, RemoteCommand{EnvVars: envVars, Args: []string{"claude", "-p", "hello"}})

	want := append(sshOptionSet(base),
		"ubuntu@"+testPublicDNS, "--",
		"env", "GH_TOKEN=tok123", "GIT_TOKEN=tok123",
		"claude", "-p", "hello",
	)
	assertCommandEquals(t, got, want)
}

func TestEC2ExecCommandArgsNeverAllocatesATTYOrWrapsInTmux(t *testing.T) {
	base := t.TempDir()

	got := BuildExecCommand(base, testPublicDNS, RemoteCommand{Args: []string{"echo", "hello"}})

	assertContainsNone(t, "exec command", strings.Join(got, " "), "-t ", "tmux")
}

func TestEC2ExecCommandArgsWithoutEnvVarsOmitsTheEnvPrefix(t *testing.T) {
	base := t.TempDir()

	got := BuildExecCommand(base, testPublicDNS, RemoteCommand{Args: []string{"echo", "hello"}})

	want := append(sshOptionSet(base), "ubuntu@"+testPublicDNS, "--", "echo", "hello")
	assertCommandEquals(t, got, want)
}

func TestEC2ExecCommandArgsSortsEnvironmentVariableKeys(t *testing.T) {
	base := t.TempDir()
	envVars := map[string]string{"ZULU": "3", "ALPHA": "1", "MIKE": "2"}

	got := BuildExecCommand(base, testPublicDNS, RemoteCommand{EnvVars: envVars, Args: []string{"env"}})

	want := append(sshOptionSet(base),
		"ubuntu@"+testPublicDNS, "--",
		"env", "ALPHA=1", "MIKE=2", "ZULU=3",
		"env",
	)
	assertCommandEquals(t, got, want)
}

func TestEC2ExecCommandArgsChangesToTheWorkdirWhenGiven(t *testing.T) {
	base := t.TempDir()

	got := BuildExecCommand(base, testPublicDNS, RemoteCommand{Workdir: "/home/ubuntu/repo", Args: []string{"git", "status"}})

	want := append(sshOptionSet(base),
		"ubuntu@"+testPublicDNS, "--",
		"cd", "/home/ubuntu/repo", "&&",
		"git", "status",
	)
	assertCommandEquals(t, got, want)
}

func TestBuildSessionExecCommandForcesATTYWithoutAHostTerminal(t *testing.T) {
	base := t.TempDir()
	envVars := map[string]string{"GH_TOKEN": "tok123"}

	got := BuildSessionExecCommand(base, testPublicDNS, RemoteCommand{
		Workdir: RemoteRepoDir,
		EnvVars: envVars,
		Args:    []string{"tmux", "new-session", "-s", "isolarium", "--", "claude"},
	})

	want := append(sshOptionSet(base),
		"-tt", "ubuntu@"+testPublicDNS, "--",
		"cd", RemoteRepoDir, "&&",
		"env", "GH_TOKEN=tok123",
		"tmux", "new-session", "-s", "isolarium", "--", "claude",
	)
	assertCommandEquals(t, got, want)
}

func TestBuildInteractiveExecCommandAllocatesATTY(t *testing.T) {
	base := t.TempDir()
	envVars := map[string]string{"GH_TOKEN": "tok123"}

	got := BuildInteractiveExecCommand(base, testPublicDNS, RemoteCommand{EnvVars: envVars, Args: []string{"claude"}})

	want := append(sshOptionSet(base),
		"-t", "ubuntu@"+testPublicDNS, "--",
		"env", "GH_TOKEN=tok123",
		"claude",
	)
	assertCommandEquals(t, got, want)
}
