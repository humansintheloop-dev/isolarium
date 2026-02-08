package lima

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
)

// buildEnvPrefix returns the "env KEY=VALUE ..." prefix for injecting environment variables.
// Returns nil if envVars is nil or empty.
func buildEnvPrefix(envVars map[string]string) []string {
	if len(envVars) == 0 {
		return nil
	}
	prefix := []string{"env"}
	// Sort keys for deterministic command output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		prefix = append(prefix, k+"="+envVars[k])
	}
	return prefix
}

// BuildExecCommand constructs the limactl command to execute a command inside the VM.
// If envVars is non-empty, the command is prefixed with "env KEY=VALUE ..." to inject
// environment variables into the VM process.
func BuildExecCommand(vm, workdir string, envVars map[string]string, args []string) []string {
	cmd := []string{"limactl", "shell", vm, "--workdir", workdir, "--"}
	cmd = append(cmd, buildEnvPrefix(envVars)...)
	cmd = append(cmd, args...)
	return cmd
}

// BuildInteractiveExecCommand constructs the limactl command for interactive execution with TTY.
// If envVars is non-empty, the command is prefixed with "env KEY=VALUE ...".
func BuildInteractiveExecCommand(vm, workdir string, envVars map[string]string, args []string) []string {
	cmd := []string{"limactl", "shell", "--tty", vm, "--workdir", workdir, "--"}
	cmd = append(cmd, buildEnvPrefix(envVars)...)
	cmd = append(cmd, args...)
	return cmd
}

// ExecCommand executes a command inside the VM in the given working directory.
// It streams stdout/stderr to the host terminal and returns the exit code.
func ExecCommand(vm, workdir string, envVars map[string]string, args []string) (int, error) {
	cmdArgs := BuildExecCommand(vm, workdir, envVars, args)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute command in VM: %w", err)
	}
	return 0, nil
}

// ExecInteractiveCommand executes a command inside the VM with TTY attached.
// It connects stdin/stdout/stderr for interactive use and returns the exit code.
func ExecInteractiveCommand(vm, workdir string, envVars map[string]string, args []string) (int, error) {
	cmdArgs := BuildInteractiveExecCommand(vm, workdir, envVars, args)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute interactive command in VM: %w", err)
	}
	return 0, nil
}
