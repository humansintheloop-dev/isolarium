package nono

func OpenShell(name string, envVars map[string]string) (int, error) {
	return runWithCommand(BuildShellCommand(), envVars, true)
}
