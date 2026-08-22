package ec2

import (
	"fmt"
	"strconv"
	"strings"
)

// statusDir is where a session's command leaves its exit status on the
// instance. The tilde is expanded by the remote shell, and by the sh that runs
// the wrapped command, so the same path reads correctly from both.
const statusDir = "~/.isolarium"

// StatusFilePath is where the command run in the named session records its exit
// status. It is deterministic per session so that a later run of the same
// command, reattaching after the first client went away, reads the same file.
func StatusFilePath(sessionName string) string {
	return statusDir + "/status-" + sessionName
}

// WrapWithExitStatus runs args under sh and writes the command's exit status to
// statusPath once it ends. The tmux client exits 0 whatever the command inside
// it did, so the status has to travel through a file. The script is quoted as
// one word for the remote login shell, with the command's own arguments quoted
// inside it for the sh that runs them.
func WrapWithExitStatus(args []string, statusPath string) []string {
	script := CommandRecord(args) + "; echo $? > " + statusPath
	return []string{"sh", "-c", shellQuote(script)}
}

// ReadExitStatus reports the exit status the command run in the named session
// recorded, once its tmux client has returned. The file is left in place so that
// a run and a reattached run of the same command both report the real status.
//
// A read the instance could not be reached for is a plain error rather than
// ErrSSHConnect: the command has already run, and a connect-aware error would
// have the caller refresh the address and run it again.
func (q InstanceQuery) ReadExitStatus(sessionName string) (int, error) {
	path := StatusFilePath(sessionName)
	output, exitCode, err := q.capture(RemoteCommand{Args: []string{"cat", path}})
	if err != nil {
		return 1, fmt.Errorf("reading the exit status at %s: %w", path, err)
	}
	if exitCode != 0 {
		return 1, fmt.Errorf("no exit status was recorded at %s on the instance (cat exited %d): the command may have been killed, or the instance rebooted", path, exitCode)
	}
	return parseExitStatus(output, path)
}

func parseExitStatus(output, path string) (int, error) {
	status, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 1, fmt.Errorf("the exit status recorded at %s on the instance is not a number: %q", path, strings.TrimSpace(output))
	}
	return status, nil
}

// ClearExitStatus removes the status a previous command left in the named
// session's file, and creates the directory the next one writes into, so a
// stale status can never be read as the new command's. It runs before the
// session starts and is the first step of a run that does not swallow errors,
// so an instance it could not reach is reported as ErrSSHConnect and the caller
// can refresh a moved instance's address and try again.
func (q InstanceQuery) ClearExitStatus(sessionName string) error {
	path := StatusFilePath(sessionName)
	_, exitCode, err := q.capture(RemoteCommand{Args: []string{"mkdir", "-p", statusDir, "&&", "rm", "-f", path}})
	if err := connectFailure(exitCode, err, sshBinary); err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("failed to clear the exit status at %s on the instance", path)
	}
	return nil
}
