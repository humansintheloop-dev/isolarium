package lima

import (
	"fmt"
	"os"
	"os/exec"
)

// BuildExecCommand constructs the limactl command to execute a command inside the VM
func BuildExecCommand(vm, workdir string, args []string) []string {
	cmd := []string{"limactl", "shell", vm, "--workdir", workdir, "--"}
	cmd = append(cmd, args...)
	return cmd
}

// BuildInteractiveExecCommand constructs the limactl command for interactive execution with TTY
func BuildInteractiveExecCommand(vm, workdir string, args []string) []string {
	cmd := []string{"limactl", "shell", "--tty", vm, "--workdir", workdir, "--"}
	cmd = append(cmd, args...)
	return cmd
}

// ExecCommand executes a command inside the VM in the given working directory.
// It streams stdout/stderr to the host terminal and returns the exit code.
func ExecCommand(vm, workdir string, args []string) (int, error) {
	cmdArgs := BuildExecCommand(vm, workdir, args)

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
func ExecInteractiveCommand(vm, workdir string, args []string) (int, error) {
	cmdArgs := BuildInteractiveExecCommand(vm, workdir, args)

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
