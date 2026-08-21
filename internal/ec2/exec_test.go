package ec2

import (
	"errors"
	"strings"
	"testing"
)

func TestEC2ExecReturnsZeroWhenTheRemoteCommandSucceeds(t *testing.T) {
	exitCode, err := runRemoteCommand([]string{"sh", "-c", "exit 0"}, false)

	if err != nil {
		t.Fatalf("runRemoteCommand() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestEC2ExecPropagatesTheRemoteExitCode(t *testing.T) {
	exitCode, err := runRemoteCommand([]string{"sh", "-c", "exit 42"}, false)

	if err != nil {
		t.Fatalf("runRemoteCommand() error = %v, want nil so the exit code is the only signal", err)
	}
	if exitCode != 42 {
		t.Errorf("exit code = %d, want 42", exitCode)
	}
}

func TestEC2ExecReportsAFailureToLaunchSSH(t *testing.T) {
	exitCode, err := runRemoteCommand([]string{"isolarium-no-such-binary"}, false)

	if err == nil {
		t.Fatal("runRemoteCommand() returned nil error for a binary that does not exist")
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(err.Error(), "isolarium-no-such-binary") {
		t.Errorf("error = %q, want it to name the command", err.Error())
	}
	if !errors.Is(err, ErrSSHConnect) {
		t.Errorf("error = %v, want a transport that never started to be reported as a connect failure", err)
	}
}

func TestEC2ExecReportsSSHsOwnErrorExitAsAConnectFailure(t *testing.T) {
	_, err := runRemoteCommand([]string{"sh", "-c", "exit 255"}, false)

	if !errors.Is(err, ErrSSHConnect) {
		t.Fatalf("error = %v, want exit code %d to be reported as a connect failure", err, sshTransportFailureExit)
	}
}

func TestEC2ExecDoesNotMistakeARejectedRemoteCommandForAConnectFailure(t *testing.T) {
	_, err := runRemoteCommand([]string{"sh", "-c", "exit 1"}, false)

	if errors.Is(err, ErrSSHConnect) {
		t.Errorf("error = %v, want a remote command the instance ran and rejected to be no connect failure", err)
	}
}
