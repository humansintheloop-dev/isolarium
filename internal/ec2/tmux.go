package ec2

import (
	"fmt"
	"io"
)

// DefaultSessionName is the one tmux session an instance runs, so that an agent
// left working on it is found again by the next connection rather than
// duplicated.
const DefaultSessionName = "isolarium"

// BuildTmuxCommand wraps args so the instance runs them inside a named tmux
// session, which outlives the SSH connection that started it. `-A` attaches to
// the session when it is already running instead of failing.
func BuildTmuxCommand(sessionName string, args []string) []string {
	return append([]string{"tmux", "new-session", "-A", "-s", sessionName, "--"}, args...)
}

// SessionExists reports whether the instance is already running the named
// session. An unreachable instance answers no, leaving the failure to the
// command that follows, which reports it far better than a probe can.
func SessionExists(session InstanceSession, sessionName string) bool {
	exitCode, err := session.exitCode(RemoteCommand{Args: []string{"tmux", "has-session", "-t", sessionName}})
	return err == nil && exitCode == 0
}

// AnnounceReattach warns that the command about to be sent will be discarded,
// which is what `tmux new-session -A` silently does when the session already
// exists. It is a notice rather than a prompt: the user's recourse is
// --new-session, not an answer.
func AnnounceReattach(errWriter io.Writer, session InstanceSession, sessionName string) {
	if !SessionExists(session, sessionName) {
		return
	}
	_, _ = fmt.Fprintf(errWriter, "attaching to existing session '%s'; use --new-session to start a fresh one\n", sessionName)
}
