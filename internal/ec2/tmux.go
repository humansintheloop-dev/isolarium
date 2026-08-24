package ec2

import (
	"fmt"
	"io"
)

// DefaultSessionName is the one tmux session an instance runs, so that an agent
// left working on it is found again by the next connection rather than
// duplicated.
const DefaultSessionName = "isolarium"

// tmuxCommandSeparator lets one tmux invocation carry two commands. It reaches
// the remote shell escaped, so the shell hands tmux a literal ';' rather than
// ending the command line there.
const tmuxCommandSeparator = `\;`

// BuildTmuxCommand wraps args so the instance runs them inside a named tmux
// session, which outlives the SSH connection that started it. `-A` attaches to
// the session when it is already running instead of failing.
func BuildTmuxCommand(sessionName string, args []string) []string {
	return append([]string{"tmux", "new-session", "-A", "-s", sessionName, "--"}, args...)
}

// BuildDetachableTmuxCommand starts a named tmux session running args, wrapped
// so its exit status is recorded, and in the same invocation records the
// command itself on the session. The record holds the unwrapped arguments,
// which is what a later run compares its own against. Without `-A` it never
// silently attaches to a session that is already running something else; the
// caller decides what to do about one.
func BuildDetachableTmuxCommand(sessionName string, args []string) []string {
	start := append([]string{"tmux", "new-session", "-s", sessionName, "--"}, WrapWithExitStatus(args, StatusFilePath(sessionName))...)
	record := []string{tmuxCommandSeparator, "set-option", "-t", sessionName, commandOption, shellQuote(CommandRecord(args))}
	return append(start, record...)
}

// BuildAttachCommand joins the named session and streams it until it ends.
func BuildAttachCommand(sessionName string) []string {
	return []string{"tmux", "attach-session", "-t", sessionName}
}

// SessionExists reports whether the instance is already running the named
// session. An unreachable instance answers no, leaving the failure to the
// command that follows, which reports it far better than a probe can. tmux's
// own complaint when there is no server to ask — the ordinary case on every
// first run — is dropped on the instance, so the probe's answer is its exit
// status alone and nothing reaches the user's terminal.
func SessionExists(session InstanceSession, sessionName string) bool {
	exitCode, err := session.exitCode(RemoteCommand{Args: []string{"tmux", "has-session", "-t", sessionName, discardRemoteStderr}})
	return err == nil && exitCode == 0
}

// discardRemoteStderr is interpreted by the remote shell, which re-parses the
// words ssh hands it, so it silences the command it follows on the instance.
const discardRemoteStderr = "2>/dev/null"

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
