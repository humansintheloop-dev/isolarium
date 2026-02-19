package nono

func BuildRunCommand(args []string) []string {
	cmd := []string{"nono", "run"}
	cmd = append(cmd, PermissionFlags()...)
	cmd = append(cmd, "--")
	cmd = append(cmd, args...)
	return cmd
}

func BuildShellCommand() []string {
	cmd := []string{"nono", "shell"}
	cmd = append(cmd, PermissionFlags()...)
	return cmd
}

func BuildRunCommandInteractive(args []string) []string {
	cmd := []string{"nono", "run"}
	cmd = append(cmd, PermissionFlags()...)
	cmd = append(cmd, "--exec")
	cmd = append(cmd, "--")
	cmd = append(cmd, args...)
	return cmd
}
