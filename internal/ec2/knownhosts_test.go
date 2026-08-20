package ec2

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

func seedKnownHosts(t *testing.T, base string) {
	t.Helper()

	if err := os.MkdirAll(EC2Dir(base), scaffoldingDirMode); err != nil {
		t.Fatalf("creating %s: %v", EC2Dir(base), err)
	}
	if err := os.WriteFile(KnownHostsPath(base), []byte("ec2-203-0-113-7.compute-1.amazonaws.com ssh-ed25519 AAAA\n"), 0600); err != nil {
		t.Fatalf("writing %s: %v", KnownHostsPath(base), err)
	}
}

func TestEvictKnownHost_RemovesTheRecordedHostFromTheDedicatedFile(t *testing.T) {
	base := t.TempDir()
	seedKnownHosts(t, base)
	runner := command.NewFakeRunner(t)
	runner.OnCommand("ssh-keygen").Returns("")

	if err := EvictKnownHost(base, "ec2-203-0-113-7.compute-1.amazonaws.com", runner); err != nil {
		t.Fatalf("EvictKnownHost() error = %v", err)
	}

	want := strings.Join([]string{"ssh-keygen", "-R", "ec2-203-0-113-7.compute-1.amazonaws.com", "-f", KnownHostsPath(base)}, " ")
	calls := runner.Calls()
	if len(calls) != 1 {
		t.Fatalf("recorded %d invocations, want 1: %v", len(calls), calls)
	}
	if got := strings.Join(calls[0], " "); got != want {
		t.Errorf("invocation =\n  %s\nwant\n  %s", got, want)
	}
}

func TestEvictKnownHost_SucceedsWhenTheKnownHostsFileIsAbsent(t *testing.T) {
	base := t.TempDir()
	runner := command.NewFakeRunner(t)

	if err := EvictKnownHost(base, "ec2-203-0-113-7.compute-1.amazonaws.com", runner); err != nil {
		t.Fatalf("EvictKnownHost() error = %v, want nil for a missing known_hosts", err)
	}
	if len(runner.Calls()) != 0 {
		t.Errorf("ran %v, want no ssh-keygen invocation", runner.Calls())
	}
}

func TestEvictKnownHost_ReportsAnSSHKeygenFailure(t *testing.T) {
	base := t.TempDir()
	seedKnownHosts(t, base)
	permissionDenied := errors.New("exit status 1")
	runner := command.NewFakeRunner(t)
	runner.OnCommand("ssh-keygen").Fails(permissionDenied)

	err := EvictKnownHost(base, "ec2-203-0-113-7.compute-1.amazonaws.com", runner)

	if !errors.Is(err, permissionDenied) {
		t.Fatalf("EvictKnownHost() error = %v, want it to wrap %v", err, permissionDenied)
	}
	if !strings.Contains(err.Error(), KnownHostsPath(base)) {
		t.Errorf("EvictKnownHost() error = %q, want it to name %q", err.Error(), KnownHostsPath(base))
	}
}
