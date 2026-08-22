package ec2

import (
	"errors"
	"strings"
	"testing"
)

func TestEC2ExitStatusReadReturnsTheStatusTheFileHolds(t *testing.T) {
	base := t.TempDir()
	reader := &sessionLister{output: "3\n"}

	got, err := NewInstanceQuery(base, testPublicDNS, reader.run).ReadExitStatus(DefaultSessionName)

	if err != nil {
		t.Fatalf("ReadExitStatus() error = %v, want nil", err)
	}
	if got != 3 {
		t.Errorf("ReadExitStatus() = %d, want 3", got)
	}
	assertCommandEquals(t, reader.command.Args, []string{"cat", "~/.isolarium/status-isolarium"})
	if reader.base != base {
		t.Errorf("ReadExitStatus() used base %q, want %q", reader.base, base)
	}
	if reader.publicDNS != testPublicDNS {
		t.Errorf("ReadExitStatus() used public DNS %q, want %q", reader.publicDNS, testPublicDNS)
	}
}

func TestEC2ExitStatusReadFailsWhenNoStatusWasRecorded(t *testing.T) {
	tests := []struct {
		name   string
		reader sessionLister
	}{
		{name: "the file is missing", reader: sessionLister{exitCode: 1, output: ""}},
		{name: "the file holds no number", reader: sessionLister{output: "not a status\n"}},
		{name: "the file is empty", reader: sessionLister{output: ""}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := tc.reader

			exitCode, err := NewInstanceQuery(t.TempDir(), testPublicDNS, reader.run).ReadExitStatus(DefaultSessionName)

			if err == nil {
				t.Fatal("ReadExitStatus() returned nil error, want the missing status reported")
			}
			if exitCode != 1 {
				t.Errorf("ReadExitStatus() exit code = %d, want 1", exitCode)
			}
			if !strings.Contains(err.Error(), "~/.isolarium/status-isolarium") {
				t.Errorf("ReadExitStatus() error = %q, want it to name the status file", err)
			}
		})
	}
}

// TestEC2ExitStatusReadNeverAsksForAnAddressRefresh pins that a read the
// instance could not be reached for is a plain error: a connect-aware one would
// have the caller retry the whole run, and the command with it.
func TestEC2ExitStatusReadNeverAsksForAnAddressRefresh(t *testing.T) {
	tests := []struct {
		name   string
		reader sessionLister
	}{
		{name: "ssh could not connect", reader: sessionLister{exitCode: 255}},
		{name: "the transport failed", reader: sessionLister{err: errors.New("ssh: not found")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := tc.reader

			_, err := NewInstanceQuery(t.TempDir(), testPublicDNS, reader.run).ReadExitStatus(DefaultSessionName)

			if err == nil {
				t.Fatal("ReadExitStatus() returned nil error for an unreachable instance")
			}
			if errors.Is(err, ErrSSHConnect) {
				t.Errorf("ReadExitStatus() error = %v, want it not to ask for a retry", err)
			}
		})
	}
}

func TestEC2ExitStatusClearRemovesTheFileAndCreatesItsDirectory(t *testing.T) {
	base := t.TempDir()
	remover := &sessionLister{}

	if err := NewInstanceQuery(base, testPublicDNS, remover.run).ClearExitStatus("isolarium-2"); err != nil {
		t.Fatalf("ClearExitStatus() error = %v, want nil", err)
	}

	assertCommandEquals(t, remover.command.Args, []string{"mkdir", "-p", "~/.isolarium", "&&", "rm", "-f", "~/.isolarium/status-isolarium-2"})
	if remover.base != base {
		t.Errorf("ClearExitStatus() used base %q, want %q", remover.base, base)
	}
	if remover.publicDNS != testPublicDNS {
		t.Errorf("ClearExitStatus() used public DNS %q, want %q", remover.publicDNS, testPublicDNS)
	}
}

// TestEC2ExitStatusClearReportsAnUnreachableInstanceAsAConnectFailure pins that
// the clear, the first step of a run that does not swallow errors, still lets
// the caller refresh a moved instance's address and try again.
func TestEC2ExitStatusClearReportsAnUnreachableInstanceAsAConnectFailure(t *testing.T) {
	tests := []struct {
		name    string
		remover sessionLister
	}{
		{name: "ssh could not connect", remover: sessionLister{exitCode: 255}},
		{name: "the transport failed", remover: sessionLister{err: errors.New("ssh: not found")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			remover := tc.remover

			err := NewInstanceQuery(t.TempDir(), testPublicDNS, remover.run).ClearExitStatus(DefaultSessionName)

			if !errors.Is(err, ErrSSHConnect) {
				t.Errorf("ClearExitStatus() error = %v, want ErrSSHConnect", err)
			}
		})
	}
}

func TestEC2ExitStatusClearReportsARejectedRemoval(t *testing.T) {
	remover := &sessionLister{exitCode: 1}

	err := NewInstanceQuery(t.TempDir(), testPublicDNS, remover.run).ClearExitStatus(DefaultSessionName)

	if err == nil {
		t.Fatal("ClearExitStatus() returned nil error for a removal the instance rejected")
	}
	if errors.Is(err, ErrSSHConnect) {
		t.Errorf("ClearExitStatus() error = %v, want a rejected removal not mistaken for a connect failure", err)
	}
	if !strings.Contains(err.Error(), "~/.isolarium/status-isolarium") {
		t.Errorf("ClearExitStatus() error = %q, want it to name the status file", err)
	}
}
