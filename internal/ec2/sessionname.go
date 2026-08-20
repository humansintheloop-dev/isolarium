package ec2

import (
	"fmt"
	"strconv"
	"strings"
)

// RemoteOutputRunner runs cmd on the instance reachable at publicDNS and returns
// what it wrote to standard output alongside the remote exit code, so a command
// the instance rejected can be told apart from one that never ran.
type RemoteOutputRunner func(base, publicDNS string, cmd RemoteCommand) (string, int, error)

// InstanceQuery is the ability to read an answer out of one instance: where its
// SSH material lives, where it can be reached, and the transport that brings the
// answer back. It is InstanceSession's counterpart for commands whose output
// matters as much as their exit status.
type InstanceQuery struct {
	base      string
	publicDNS string
	run       RemoteOutputRunner
}

func NewInstanceQuery(base, publicDNS string, run RemoteOutputRunner) InstanceQuery {
	return InstanceQuery{base: base, publicDNS: publicDNS, run: run}
}

// mustRun turns a non-zero remote exit code into an error, because a step of a
// multi-command operation has no exit status worth propagating — it either
// happened or the operation cannot continue. It is InstanceSession.mustRun for
// the transport that also carries output back.
func (q InstanceQuery) mustRun(cmd RemoteCommand, description string) error {
	_, exitCode, err := q.run(q.base, q.publicDNS, cmd)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("failed to %s on the instance", description)
	}
	return nil
}

// ListSessions reports the tmux sessions the instance is running. A non-zero
// exit means no tmux server is running there, which is no sessions rather than a
// failure. The format string is quoted because the remote shell would otherwise
// read its leading '#' as the start of a comment.
func (q InstanceQuery) ListSessions() ([]string, error) {
	output, exitCode, err := q.run(q.base, q.publicDNS, RemoteCommand{
		Args: []string{"tmux", "list-sessions", "-F", shellQuote("#{session_name}")},
	})
	if err != nil {
		return nil, err
	}
	if exitCode != 0 {
		return nil, nil
	}

	var names []string
	for _, line := range strings.Split(output, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// ResolveNewSessionName asks the instance which sessions it is running and
// answers with one that is free.
func (q InstanceQuery) ResolveNewSessionName() (string, error) {
	existing, err := q.ListSessions()
	if err != nil {
		return "", err
	}
	return NextSessionName(existing), nil
}

// NextSessionName picks a session name none of the running sessions holds, so
// --new-session adds a session rather than displacing the agent working in one.
// Numbering starts at 2 and fills the lowest gap, so closing a session makes its
// number available again.
func NextSessionName(existing []string) string {
	running := make(map[string]bool, len(existing))
	for _, name := range existing {
		running[name] = true
	}
	if !running[DefaultSessionName] {
		return DefaultSessionName
	}
	for number := 2; ; number++ {
		candidate := DefaultSessionName + "-" + strconv.Itoa(number)
		if !running[candidate] {
			return candidate
		}
	}
}
