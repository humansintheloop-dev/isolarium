package lima

import (
	"fmt"
	"os"
	"os/exec"
)

// BuildShellCommand constructs the limactl command to open an interactive shell
func BuildShellCommand(vm string) []string {
	return []string{"limactl", "shell", "--tty", vm}
}

// OpenShell opens an interactive shell inside the VM
func OpenShell(vm string) (int, error) {
	cmdArgs := BuildShellCommand(vm)

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
