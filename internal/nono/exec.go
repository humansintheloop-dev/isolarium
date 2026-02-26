package nono

import (
	"fmt"
	"os"
	"os/exec"
)

func ExecCommand(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
	return runWithCommand(BuildRunCommand(args, extraReadPaths), envVars)
}

func ExecInteractiveCommand(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
	return runWithCommand(BuildRunCommandInteractive(args, extraReadPaths), envVars)
}

func runWithCommand(cmdArgs []string, envVars map[string]string) (int, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = buildEnv(envVars)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute command in nono sandbox: %w", err)
	}
	return 0, nil
}

func buildEnv(envVars map[string]string) []string {
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	return env
}
