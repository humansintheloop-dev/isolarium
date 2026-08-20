package ec2

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

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

// runRemoteCommand streams the command's stdio and maps a non-zero remote exit
// into an exit code rather than an error, so callers can propagate it verbatim.
func runRemoteCommand(cmdArgs []string, interactive bool) (int, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if interactive {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, fmt.Errorf("failed to run %s on the instance: %w", cmdArgs[0], err)
}
