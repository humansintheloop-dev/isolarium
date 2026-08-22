package ec2

const sshBinary = "ssh"

// ttyMode is what ssh is asked to do about a remote pseudo-terminal.
type ttyMode int

const (
	// noTTY runs the command plainly, so its exit code and stdout stay faithful.
	noTTY ttyMode = iota
	// requestTTY asks for a remote terminal when the host has one, which is what
	// an interactive command needs.
	requestTTY
	// forceTTY allocates a remote terminal even when the host has none, which is
	// what tmux needs when isolarium itself runs without a terminal.
	forceTTY
)

func (m ttyMode) sshFlags() []string {
	switch m {
	case requestTTY:
		return []string{"-t"}
	case forceTTY:
		return []string{"-tt"}
	default:
		return nil
	}
}

// buildSSHArgs is the single source of the SSH option set, so Exec,
// ExecInteractive, OpenShell, and CopyCredentials cannot drift apart. Only the
// terminal handling differs between them.
func buildSSHArgs(base, publicDNS string, tty ttyMode) []string {
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
	args = append(args, tty.sshFlags()...)
	return append(args, RemoteUser+"@"+publicDNS)
}

// BuildExecCommand constructs the ssh command that runs cmd on the instance
// without a TTY and without tmux, so the exit code and stdout stay faithful.
func BuildExecCommand(base, publicDNS string, cmd RemoteCommand) []string {
	return buildSSHCommand(base, publicDNS, cmd, noTTY)
}

// BuildInteractiveExecCommand constructs the ssh command that runs cmd on the
// instance with a TTY attached.
func BuildInteractiveExecCommand(base, publicDNS string, cmd RemoteCommand) []string {
	return buildSSHCommand(base, publicDNS, cmd, requestTTY)
}

// BuildSessionExecCommand constructs the ssh command for a cmd that must run
// inside tmux on the instance while isolarium itself may have no terminal: the
// pseudo-terminal is forced so tmux can start, and nothing is read from the
// host.
func BuildSessionExecCommand(base, publicDNS string, cmd RemoteCommand) []string {
	return buildSSHCommand(base, publicDNS, cmd, forceTTY)
}

func buildSSHCommand(base, publicDNS string, cmd RemoteCommand, tty ttyMode) []string {
	ssh := append(buildSSHArgs(base, publicDNS, tty), "--")
	return append(ssh, cmd.shellWords()...)
}
