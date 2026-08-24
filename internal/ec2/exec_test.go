package ec2

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestEC2ExecReturnsZeroWhenTheRemoteCommandSucceeds(t *testing.T) {
	exitCode, err := runRemoteCommand([]string{"sh", "-c", "exit 0"}, disconnectedStdin)

	if err != nil {
		t.Fatalf("runRemoteCommand() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestEC2ExecPropagatesTheRemoteExitCode(t *testing.T) {
	exitCode, err := runRemoteCommand([]string{"sh", "-c", "exit 42"}, disconnectedStdin)

	if err != nil {
		t.Fatalf("runRemoteCommand() error = %v, want nil so the exit code is the only signal", err)
	}
	if exitCode != 42 {
		t.Errorf("exit code = %d, want 42", exitCode)
	}
}

func TestEC2ExecInSessionLeavesTheHostStdinDisconnected(t *testing.T) {
	process := remoteProcess([]string{"sh", "-c", "exit 0"}, disconnectedStdin)

	if process.Stdin != nil {
		t.Errorf("session process stdin = %v, want it disconnected so tmux starts without a host terminal", process.Stdin)
	}
	if process.Stdout != os.Stdout || process.Stderr != os.Stderr {
		t.Error("session process does not stream stdout and stderr to the host")
	}
}

// The host's stdin is read when the command is launched rather than when the
// package loads, because the ec2 suite hands its interactive connections a
// pseudo-terminal by swapping os.Stdin for the duration of a test.
func TestEC2ExecInteractiveConnectsTheStdinTheHostHasNow(t *testing.T) {
	swapped := swapHostStdin(t)

	process := remoteProcess([]string{"sh", "-c", "exit 0"}, hostTerminalStdin())

	if process.Stdin != swapped {
		t.Errorf("interactive process stdin = %v, want the host's current stdin %v", process.Stdin, swapped)
	}
}

func swapHostStdin(t *testing.T) *os.File {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating a stdin pipe: %v", err)
	}
	original := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = original
		_ = reader.Close()
		_ = writer.Close()
	})
	return reader
}

func TestEC2ExecFeedsAScriptToTheTransportsStdin(t *testing.T) {
	script := RemoteCommand{Args: []string{"sh", "-s"}, Stdin: "exit 7\n"}

	exitCode, err := runRemoteCommand([]string{"sh", "-s"}, script.hostStdin())

	if err != nil {
		t.Fatalf("runRemoteCommand() error = %v", err)
	}
	if exitCode != 7 {
		t.Errorf("exit code = %d, want 7 from the script the command carried on stdin", exitCode)
	}
}

func TestEC2ExecLeavesStdinDisconnectedForACommandThatCarriesNone(t *testing.T) {
	if got := (RemoteCommand{Args: []string{"true"}}).hostStdin(); got != disconnectedStdin {
		t.Errorf("hostStdin() = %v, want it disconnected", got)
	}
}

func TestEC2ExecReportsAFailureToLaunchSSH(t *testing.T) {
	exitCode, err := runRemoteCommand([]string{"isolarium-no-such-binary"}, disconnectedStdin)

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
	_, err := runRemoteCommand([]string{"sh", "-c", "exit 255"}, disconnectedStdin)

	if !errors.Is(err, ErrSSHConnect) {
		t.Fatalf("error = %v, want exit code %d to be reported as a connect failure", err, sshTransportFailureExit)
	}
}

func TestEC2ExecDoesNotMistakeARejectedRemoteCommandForAConnectFailure(t *testing.T) {
	_, err := runRemoteCommand([]string{"sh", "-c", "exit 1"}, disconnectedStdin)

	if errors.Is(err, ErrSSHConnect) {
		t.Errorf("error = %v, want a remote command the instance ran and rejected to be no connect failure", err)
	}
}
