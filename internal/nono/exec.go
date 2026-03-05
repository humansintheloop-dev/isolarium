package nono

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func ExecCommand(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
	sc := sandboxCommand{args: BuildRunCommand(args, extraReadPaths), envVars: envVars}
	return sc.run()
}

func ExecInteractiveCommand(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
	sc := sandboxCommand{args: BuildRunCommandInteractive(args, extraReadPaths), envVars: envVars, interactive: true}
	return sc.run()
}

type sandboxCommand struct {
	args        []string
	envVars     map[string]string
	interactive bool
}

func (sc sandboxCommand) run() (int, error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	return runWithSignals(sc, sigCh, 10*time.Second)
}

func (sc sandboxCommand) build() *exec.Cmd {
	fmt.Fprintf(os.Stderr, "DEBUG nono command: %s\n", strings.Join(sc.args, " "))
	cmd := exec.Command(sc.args[0], sc.args[1:]...)
	cmd.Env = buildEnv(sc.envVars)
	if sc.interactive {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if !sc.interactive {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}

func runWithSignals(sc sandboxCommand, sigCh <-chan os.Signal, gracePeriod time.Duration) (int, error) {
	cmd := sc.build()

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start command in nono sandbox: %w", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	killPid := -cmd.Process.Pid
	if sc.interactive {
		killPid = cmd.Process.Pid
	}

	select {
	case sig := <-sigCh:
		_ = syscall.Kill(killPid, sig.(syscall.Signal))
		escalateOrWait(killPid, sigCh, doneCh, gracePeriod)
		return 128 + int(sig.(syscall.Signal)), nil
	case err := <-doneCh:
		return exitCodeFromError(err)
	}
}

func escalateOrWait(killPid int, sigCh <-chan os.Signal, doneCh <-chan error, gracePeriod time.Duration) {
	select {
	case <-doneCh:
	case <-sigCh:
		_ = syscall.Kill(killPid, syscall.SIGKILL)
		<-doneCh
	case <-time.After(gracePeriod):
		_ = syscall.Kill(killPid, syscall.SIGKILL)
		<-doneCh
	}
}

func exitCodeFromError(err error) (int, error) {
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute command in nono sandbox: %w", err)
	}
	return 0, nil
}

func buildEnv(envVars map[string]string) []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			env = append(env, e)
		}
	}
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	return env
}
