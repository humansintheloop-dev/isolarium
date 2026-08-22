package ec2

import (
	"errors"
	"fmt"
	"os/exec"
)

// ErrSSHConnect marks a failure to reach the instance at all — ssh's own error
// exit or a transport that never started — as distinct from a remote command the
// instance ran and rejected. Only the former is worth refreshing an address for.
var ErrSSHConnect = errors.New("ssh could not connect to the instance")

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
