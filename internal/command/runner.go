package command

import (
	"bytes"
	"io"
	"os/exec"
)

type Runner interface {
	Run(name string, args ...string) ([]byte, error)

	// RunStreaming is Run for a command whose output a human should see while it
	// is still working. A command long enough to look hung — terraform holding a
	// destroy open while it retries — is indistinguishable from a deadlock when
	// its only output arrives after it exits.
	RunStreaming(out io.Writer, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

// Run collects the output and reveals none of it until the command exits, which
// is what a caller handling secrets or parsing the result wants.
func (r ExecRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r ExecRunner) RunStreaming(out io.Writer, name string, args ...string) ([]byte, error) {
	if out == nil {
		return r.Run(name, args...)
	}

	var collected bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.MultiWriter(&collected, out)
	cmd.Stderr = cmd.Stdout

	err := cmd.Run()
	return collected.Bytes(), err
}
