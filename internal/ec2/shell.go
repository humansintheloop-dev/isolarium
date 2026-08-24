package ec2

// loginShell is what `isolarium shell` drops the user into. The login flags make
// it read the profile that first-boot provisioning put the toolchain's PATH in.
var loginShell = []string{"bash", "-il"}

// ShellCommand is a login shell inside the instance's tmux session, rooted at
// the repository create placed there.
func ShellCommand(sessionName string, envVars map[string]string) RemoteCommand {
	return RemoteCommand{
		Workdir: RemoteRepoDir,
		EnvVars: envVars,
		Args:    BuildTmuxCommand(sessionName, loginShell),
	}
}

// OpenShell attaches the host terminal to a login shell on the instance and
// returns the shell's exit code.
func OpenShell(session InstanceSession, sessionName string, envVars map[string]string) (int, error) {
	return session.exitCode(ShellCommand(sessionName, envVars))
}
