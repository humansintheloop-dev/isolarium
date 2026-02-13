package docker

import (
	"fmt"
	"os"
	"os/exec"
)

func BuildShellCommand(name string, envVars map[string]string) []string {
	cmd := []string{"docker", "exec", "-it"}
	cmd = append(cmd, buildEnvFlags(envVars)...)
	cmd = append(cmd, "-w", "/home/isolarium/repo")
	cmd = append(cmd, name)
	cmd = append(cmd, "bash")
	return cmd
}

func OpenShell(name string, envVars map[string]string) (int, error) {
	cmdArgs := BuildShellCommand(name, envVars)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to open shell in container: %w", err)
	}
	return 0, nil
}
