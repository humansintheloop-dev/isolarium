package ec2

import "sort"

const sshBinary = "ssh"

// RemoteCommand is what the instance should run: the command itself, the
// directory it runs from, and the environment it sees.
type RemoteCommand struct {
	Workdir string
	EnvVars map[string]string
	Args    []string
}

// BuildSSHArgs is the single source of the SSH option set, so Exec,
// ExecInteractive, OpenShell, and CopyCredentials cannot drift apart. A TTY is
// requested only by the interactive paths.
func BuildSSHArgs(base, publicDNS string, tty bool) []string {
	args := []string{
		sshBinary,
		"-i", PrivateKeyPath(base),
		"-o", "UserKnownHostsFile=" + KnownHostsPath(base),
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "IdentitiesOnly=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=4",
	}
	if tty {
		args = append(args, "-t")
	}
	return append(args, RemoteUser+"@"+publicDNS)
}

// BuildExecCommand constructs the ssh command that runs cmd on the instance
// without a TTY and without tmux, so the exit code and stdout stay faithful.
func BuildExecCommand(base, publicDNS string, cmd RemoteCommand) []string {
	return buildSSHCommand(base, publicDNS, cmd, false)
}

// BuildInteractiveExecCommand constructs the ssh command that runs cmd on the
// instance with a TTY attached.
func BuildInteractiveExecCommand(base, publicDNS string, cmd RemoteCommand) []string {
	return buildSSHCommand(base, publicDNS, cmd, true)
}

func buildSSHCommand(base, publicDNS string, cmd RemoteCommand, tty bool) []string {
	ssh := append(BuildSSHArgs(base, publicDNS, tty), "--")
	ssh = append(ssh, changeDirectoryPrefix(cmd.Workdir)...)
	ssh = append(ssh, buildEnvPrefix(cmd.EnvVars)...)
	return append(ssh, cmd.Args...)
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
