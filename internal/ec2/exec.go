package ec2

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ErrSSHConnect marks a failure to reach the instance at all — ssh's own error
// exit or a transport that never started — as distinct from a remote command the
// instance ran and rejected. Only the former is worth refreshing an address for.
var ErrSSHConnect = errors.New("ssh could not connect to the instance")

// ExecCommand runs cmd on the instance over SSH, streaming stdout and stderr to
// the host terminal, and returns the remote command's exit code.
func ExecCommand(base, publicDNS string, cmd RemoteCommand) (int, error) {
	return runRemoteCommand(BuildExecCommand(base, publicDNS, cmd), false)
}

// ExecInteractiveCommand runs cmd on the instance over SSH with a TTY attached,
// connecting stdin as well, and returns the remote command's exit code.
func ExecInteractiveCommand(base, publicDNS string, cmd RemoteCommand) (int, error) {
	return runRemoteCommand(BuildInteractiveExecCommand(base, publicDNS, cmd), true)
}

// CaptureCommand runs cmd on the instance over SSH and returns what it wrote to
// standard output, so isolarium can read an answer out of the instance rather
// than only relaying it to the terminal. Stderr still reaches the host, because
// a remote diagnostic is worth seeing whichever way the command is run.
func CaptureCommand(base, publicDNS string, cmd RemoteCommand) (string, int, error) {
	cmdArgs := BuildExecCommand(base, publicDNS, cmd)
	command := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = os.Stderr

	exitCode, err := remoteExitCode(command.Run(), cmdArgs[0])
	return stdout.String(), exitCode, err
}

// runRemoteCommand streams the command's stdio and maps a non-zero remote exit
// into an exit code rather than an error, so callers can propagate it verbatim.
func runRemoteCommand(cmdArgs []string, interactive bool) (int, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if interactive {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return connectAwareExitCode(cmd.Run(), cmdArgs[0])
}

// connectAwareExitCode reports a run whose command never reached the instance as
// ErrSSHConnect, leaving a remote exit code to travel back on its own as before.
func connectAwareExitCode(err error, binary string) (int, error) {
	exitCode, runErr := remoteExitCode(err, binary)
	if runErr != nil {
		return exitCode, fmt.Errorf("%w: %v", ErrSSHConnect, runErr)
	}
	if exitCode == sshTransportFailureExit {
		return exitCode, fmt.Errorf("%w: %s exited %d", ErrSSHConnect, binary, sshTransportFailureExit)
	}
	return exitCode, nil
}

// remoteExitCode separates a command the instance ran and rejected, which has an
// exit status worth propagating, from one the host could not launch at all.
func remoteExitCode(err error, binary string) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, fmt.Errorf("failed to run %s on the instance: %w", binary, err)
}
