package nono

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func ExecCommand(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
	return runWithCommand(BuildRunCommand(args, extraReadPaths), envVars)
}

func ExecInteractiveCommand(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
	return runWithCommand(BuildRunCommandInteractive(args, extraReadPaths), envVars)
}

func runWithCommand(cmdArgs []string, envVars map[string]string) (int, error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	return runWithSignals(cmdArgs, envVars, sigCh, 10*time.Second)
}

func runWithSignals(cmdArgs []string, envVars map[string]string, sigCh <-chan os.Signal, gracePeriod time.Duration) (int, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = buildEnv(envVars)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start command in nono sandbox: %w", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	select {
	case sig := <-sigCh:
		_ = syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		<-doneCh
		return 128 + int(sig.(syscall.Signal)), nil
	case err := <-doneCh:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), nil
			}
			return 1, fmt.Errorf("failed to execute command in nono sandbox: %w", err)
		}
		return 0, nil
	}
}

func buildEnv(envVars map[string]string) []string {
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	return env
}
