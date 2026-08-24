package ec2

import (
	"fmt"
	"regexp"
	"strings"
)

// commandOption is the tmux user option a session started by `isolarium run`
// carries, holding the command it runs so that a later run can tell whether it
// is asking for that same command.
const commandOption = "@isolarium-command"

// shellWord matches an argument the remote shell would pass through unchanged,
// so the record stays readable for everything but the arguments that need
// quoting.
var shellWord = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,-]+$`)

// CommandRecord is how a command's arguments are written on its tmux session,
// and how a later run renders its own arguments to compare against them.
func CommandRecord(args []string) string {
	words := make([]string, len(args))
	for i, arg := range args {
		words[i] = quoteUnlessShellWord(arg)
	}
	return strings.Join(words, " ")
}

func quoteUnlessShellWord(arg string) string {
	if shellWord.MatchString(arg) {
		return arg
	}
	return shellQuote(arg)
}

// RecordedCommand reads back what the named session was started to run. A
// session that recorded nothing — one started interactively, or by something
// other than isolarium — answers with an empty record rather than an error.
func (q InstanceQuery) RecordedCommand(sessionName string) (string, error) {
	output, exitCode, err := q.capture(RemoteCommand{
		Args: []string{"tmux", "show-option", "-qv", "-t", sessionName, commandOption},
	})
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", nil
	}
	return strings.TrimSpace(output), nil
}

// SessionBusyError explains why args were not run: the session is already
// running something else, and the ways back in are to join it or to start
// another session beside it.
func SessionBusyError(sessionName, recorded string, args []string) error {
	return fmt.Errorf("cannot run '%s': session '%s' on the instance is already running %s; "+
		"reattach to it with isolarium shell --type ec2, or start another session with --new-session",
		CommandRecord(args), sessionName, describeRecordedCommand(recorded))
}

// describeRecordedCommand names what a session runs, allowing for one that was
// started interactively and so recorded nothing.
func describeRecordedCommand(recorded string) string {
	if recorded == "" {
		return "a command isolarium run did not start"
	}
	return "'" + recorded + "'"
}
