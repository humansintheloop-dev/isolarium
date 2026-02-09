package lima

import (
	"fmt"
	"os"
	"os/exec"
)

// BuildShellCommand constructs the limactl command to open an interactive shell.
// When envVars is non-empty, the command injects environment variables via
// "env KEY=VALUE ... bash -il" so tools like gh and git can authenticate.
func BuildShellCommand(vm string, envVars map[string]string) []string {
	cmd := []string{"limactl", "shell", "--tty", vm}
	envPrefix := buildEnvPrefix(envVars)
	if len(envPrefix) > 0 {
		cmd = append(cmd, "--")
		cmd = append(cmd, envPrefix...)
		cmd = append(cmd, "bash", "-il")
	}
	return cmd
}

func OpenShell(vm string, envVars map[string]string) (int, error) {
	cmdArgs := BuildShellCommand(vm, envVars)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to open shell in VM: %w", err)
	}
	return 0, nil
}
