package ec2

import "testing"

func TestEC2TmuxShellRunsALoginShellInTheRepository(t *testing.T) {
	base := t.TempDir()
	transport := &sessionProbe{exitCode: 3}

	exitCode, err := OpenShell(NewInstanceSession(base, testPublicDNS, transport.run), DefaultSessionName, map[string]string{"GH_TOKEN": "tok123"})

	if err != nil {
		t.Fatalf("OpenShell() error = %v, want nil", err)
	}
	if exitCode != 3 {
		t.Errorf("OpenShell() exit code = %d, want 3", exitCode)
	}
	if transport.command.Workdir != RemoteRepoDir {
		t.Errorf("OpenShell() workdir = %q, want %q", transport.command.Workdir, RemoteRepoDir)
	}
	if transport.command.EnvVars["GH_TOKEN"] != "tok123" {
		t.Errorf("OpenShell() passed GH_TOKEN %q, want %q", transport.command.EnvVars["GH_TOKEN"], "tok123")
	}
	assertCommandEquals(t, transport.command.Args, []string{"tmux", "new-session", "-A", "-s", "isolarium", "--", "bash", "-il"})
}

func TestEC2TmuxShellTravelsOverTheInteractiveTransport(t *testing.T) {
	base := t.TempDir()

	got := BuildInteractiveExecCommand(base, testPublicDNS, ShellCommand(DefaultSessionName, nil))

	want := append(sshOptionSet(base),
		"-t", "ubuntu@"+testPublicDNS, "--",
		"cd", RemoteRepoDir, "&&",
		"tmux", "new-session", "-A", "-s", "isolarium", "--", "bash", "-il",
	)
	assertCommandEquals(t, got, want)
}
