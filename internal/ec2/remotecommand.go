package ec2

import (
	"io"
	"sort"
	"strings"
)

// RemoteCommand is what the instance should run: the command itself, the
// directory it runs from, the environment it sees, and what it reads on
// standard input.
type RemoteCommand struct {
	Workdir string
	EnvVars map[string]string
	Args    []string
	// Stdin is fed to the command on the instance, which is how a script
	// travels to `bash -s` without touching the instance's disk. Empty leaves
	// standard input disconnected.
	Stdin string
}

// hostStdin is what the host process that carries the command should read from:
// the script the command brings with it, or nothing.
func (cmd RemoteCommand) hostStdin() io.Reader {
	if cmd.Stdin == "" {
		return disconnectedStdin
	}
	return strings.NewReader(cmd.Stdin)
}

// shellWords is the command as the remote login shell should read it: a change
// of directory when one is asked for, the environment, then the command.
func (cmd RemoteCommand) shellWords() []string {
	words := append([]string{}, changeDirectoryPrefix(cmd.Workdir)...)
	words = append(words, buildEnvPrefix(cmd.EnvVars)...)
	return append(words, cmd.Args...)
}

// changeDirectoryPrefix returns the remote-shell prefix that runs the command
// from workdir. An empty workdir leaves the command in the login directory.
func changeDirectoryPrefix(workdir string) []string {
	if workdir == "" {
		return nil
	}
	return []string{"cd", workdir, "&&"}
}

// buildEnvPrefix returns the "env KEY=VALUE ..." prefix that injects environment
// variables into the remote process, with keys sorted so the command is
// deterministic. Returns nil when envVars is nil or empty.
func buildEnvPrefix(envVars map[string]string) []string {
	if len(envVars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	prefix := []string{"env"}
	for _, key := range keys {
		prefix = append(prefix, key+"="+envVars[key])
	}
	return prefix
}
