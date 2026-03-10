package command

import "os/exec"

type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (r ExecRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
