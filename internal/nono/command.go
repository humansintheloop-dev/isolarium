package nono

func BuildRunCommand(args []string, extraReadPaths []string) []string {
	cmd := []string{"nono", "wrap", "--profile", getProfilePath()}
	cmd = append(cmd, PermissionFlags()...)
	for _, p := range extraReadPaths {
		cmd = append(cmd, "--read", p)
	}
	cmd = append(cmd, "--")
	cmd = append(cmd, args...)
	return cmd
}

func BuildShellCommand() []string {
	cmd := []string{"nono", "shell", "--profile", getProfilePath()}
	cmd = append(cmd, PermissionFlags()...)
	return cmd
}

func BuildRunCommandInteractive(args []string, extraReadPaths []string) []string {
	cmd := []string{"nono", "run", "--profile", getProfilePath()}
	cmd = append(cmd, PermissionFlags()...)
	for _, p := range extraReadPaths {
		cmd = append(cmd, "--read", p)
	}
	cmd = append(cmd, "--exec")
	cmd = append(cmd, "--")
	cmd = append(cmd, args...)
	return cmd
}
