package docker

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
)

func buildEnvFlags(envVars map[string]string) []string {
	if len(envVars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var flags []string
	for _, k := range keys {
		flags = append(flags, "-e", k+"="+envVars[k])
	}
	return flags
}

func BuildExecCommand(name string, envVars map[string]string, args []string) []string {
	cmd := []string{"docker", "exec"}
	cmd = append(cmd, buildEnvFlags(envVars)...)
	cmd = append(cmd, name)
	cmd = append(cmd, args...)
	return cmd
}

func BuildInteractiveExecCommand(name string, envVars map[string]string, args []string) []string {
	cmd := []string{"docker", "exec", "-it"}
	cmd = append(cmd, buildEnvFlags(envVars)...)
	cmd = append(cmd, name)
	cmd = append(cmd, args...)
	return cmd
}

func ExecCommand(name string, envVars map[string]string, args []string) (int, error) {
	cmdArgs := BuildExecCommand(name, envVars, args)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute command in container: %w", err)
	}
	return 0, nil
}

func ExecInteractiveCommand(name string, envVars map[string]string, args []string) (int, error) {
	cmdArgs := BuildInteractiveExecCommand(name, envVars, args)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute interactive command in container: %w", err)
	}
	return 0, nil
}
