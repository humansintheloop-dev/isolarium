package command

import (
	"strings"
	"testing"
)

type FakeRunner struct {
	t        *testing.T
	commands map[string]*commandResponse
	executed map[string]bool
}

type commandResponse struct {
	output []byte
	err    error
}

func NewFakeRunner(t *testing.T) *FakeRunner {
	return &FakeRunner{t: t, commands: make(map[string]*commandResponse), executed: make(map[string]bool)}
}

type commandExpectation struct {
	key    string
	runner *FakeRunner
}

func (f *FakeRunner) OnCommand(args ...string) *commandExpectation {
	key := strings.Join(args, "\x00")
	return &commandExpectation{key: key, runner: f}
}

func (e *commandExpectation) Returns(output string) {
	e.runner.commands[e.key] = &commandResponse{output: []byte(output)}
}

func (e *commandExpectation) Fails(err error) {
	e.runner.commands[e.key] = &commandResponse{err: err}
}

func (f *FakeRunner) Run(name string, args ...string) ([]byte, error) {
	actual := append([]string{name}, args...)
	key := strings.Join(actual, "\x00")
	if resp, ok := f.commands[key]; ok {
		f.executed[key] = true
		return resp.output, resp.err
	}
	// Try matching by command name only (prefix match)
	for k, resp := range f.commands {
		parts := strings.Split(k, "\x00")
		if parts[0] == name {
			f.executed[k] = true
			return resp.output, resp.err
		}
	}
	f.t.Fatalf("unexpected command: %s %s", name, strings.Join(args, " "))
	return nil, nil
}

func (f *FakeRunner) VerifyExecuted() {
	f.t.Helper()
	for key := range f.commands {
		if !f.executed[key] {
			parts := strings.Split(key, "\x00")
			f.t.Errorf("expected command was never called: %s", strings.Join(parts, " "))
		}
	}
}
