package ec2

import (
	"bytes"
	"io"
	"os"
	"os/exec"
)

// The host's standard input travels to the instance only when a command asks
// for it.
var (
	// disconnectedStdin sends nothing from the host, which is what a
	// non-interactive command wants and what lets a forced pseudo-terminal start
	// tmux when isolarium itself has no terminal.
	disconnectedStdin io.Reader = nil
	// connectedStdin lets the user type into the remote command.
	connectedStdin io.Reader = os.Stdin
)

// ExecCommand runs cmd on the instance over SSH, streaming stdout and stderr to
// the host terminal and feeding the command whatever input it carries, and
// returns the remote command's exit code.
func ExecCommand(base, publicDNS string, cmd RemoteCommand) (int, error) {
	return runRemoteCommand(BuildExecCommand(base, publicDNS, cmd), cmd.hostStdin())
}

// ExecInteractiveCommand runs cmd on the instance over SSH with a TTY attached,
// connecting stdin as well, and returns the remote command's exit code.
func ExecInteractiveCommand(base, publicDNS string, cmd RemoteCommand) (int, error) {
	return runRemoteCommand(BuildInteractiveExecCommand(base, publicDNS, cmd), connectedStdin)
}

// ExecInSessionCommand runs a tmux-wrapped cmd on the instance over SSH with a
// forced pseudo-terminal, streaming stdout and stderr to the host but sending
// nothing from it, so the session starts even when isolarium has no terminal
// and keeps running if the connection drops.
func ExecInSessionCommand(base, publicDNS string, cmd RemoteCommand) (int, error) {
	return runRemoteCommand(BuildSessionExecCommand(base, publicDNS, cmd), disconnectedStdin)
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
func runRemoteCommand(cmdArgs []string, stdin io.Reader) (int, error) {
	return connectAwareExitCode(remoteProcess(cmdArgs, stdin).Run(), cmdArgs[0])
}

// remoteProcess is the host process that carries the command to the instance,
// streaming what comes back to the host terminal.
func remoteProcess(cmdArgs []string, stdin io.Reader) *exec.Cmd {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}
