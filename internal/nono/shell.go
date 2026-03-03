package nono

func OpenShell(name string, envVars map[string]string) (int, error) {
	sc := sandboxCommand{args: BuildShellCommand(), envVars: envVars, interactive: true}
	return sc.run()
}
