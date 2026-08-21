package command

import (
	"io"
	"slices"
	"strings"
	"testing"
)

type FakeRunner struct {
	t        *testing.T
	commands map[commandKey]*commandResponse
	executed map[commandKey]bool
	calls    [][]string
}

type commandResponse struct {
	output []byte
	err    error
}

func NewFakeRunner(t *testing.T) *FakeRunner {
	return &FakeRunner{t: t, commands: make(map[commandKey]*commandResponse), executed: make(map[commandKey]bool)}
}

// commandLine is one invocation, whether registered or actually run. It carries
// its own map key so a registration and a call are always joined and split the
// same way.
type commandLine []string

const commandLineSeparator = "\x00"

// commandKey is a commandLine flattened into something a map can hold. It is its
// own type so that only key() can produce one, rather than any string at hand.
type commandKey string

func (c commandLine) key() commandKey {
	return commandKey(strings.Join(c, commandLineSeparator))
}

func commandLineFromKey(key commandKey) commandLine {
	return strings.Split(string(key), commandLineSeparator)
}

func (c commandLine) String() string {
	return strings.Join(c, " ")
}

func (c commandLine) sharedPrefixWith(other commandLine) int {
	shared := 0
	for shared < min(len(c), len(other)) && c[shared] == other[shared] {
		shared++
	}
	return shared
}

type commandExpectation struct {
	key    commandKey
	runner *FakeRunner
}

func (f *FakeRunner) OnCommand(args ...string) *commandExpectation {
	return &commandExpectation{key: commandLine(args).key(), runner: f}
}

func (e *commandExpectation) Returns(output string) {
	e.runner.commands[e.key] = &commandResponse{output: []byte(output)}
}

func (e *commandExpectation) Fails(err error) {
	e.runner.commands[e.key] = &commandResponse{err: err}
}

func (f *FakeRunner) Run(name string, args ...string) ([]byte, error) {
	actual := commandLine(append([]string{name}, args...))
	f.calls = append(f.calls, actual)

	if key, ok := f.bestMatch(actual); ok {
		f.executed[key] = true
		return f.commands[key].output, f.commands[key].err
	}
	f.t.Fatalf("unexpected command: %s", actual)
	return nil, nil
}

// bestMatch resolves which registration answers a call. Registrations range from
// a whole tool stubbed by name to one exact invocation, so the most specific one
// that the call is consistent with wins.
func (f *FakeRunner) bestMatch(actual commandLine) (commandKey, bool) {
	candidates := f.matchesFor(actual)
	if len(candidates) == 0 {
		return "", false
	}
	return slices.MaxFunc(candidates, match.compare).key, true
}

func (f *FakeRunner) matchesFor(actual commandLine) []match {
	var matches []match
	for key := range f.commands {
		registered := commandLineFromKey(key)
		if registered[0] != actual[0] {
			continue
		}
		matches = append(matches, registered.matching(actual))
	}
	return matches
}

func (c commandLine) matching(actual commandLine) match {
	shared := c.sharedPrefixWith(actual)
	return match{key: c.key(), extendedByCall: shared == len(c), sharedArguments: shared}
}

// match is one registration's fitness for a call: a registration the call fully
// extends outranks one that merely shares a leading run of arguments, and among
// equals the longer shared run wins.
type match struct {
	key             commandKey
	extendedByCall  bool
	sharedArguments int
}

// compare falls back to the key so that equally fit registrations resolve the
// same way every run, rather than however map iteration happened to order them.
func (m match) compare(other match) int {
	if m.extendedByCall != other.extendedByCall {
		return rankOfExtension(m.extendedByCall)
	}
	if m.sharedArguments != other.sharedArguments {
		return m.sharedArguments - other.sharedArguments
	}
	return strings.Compare(string(other.key), string(m.key))
}

func rankOfExtension(extendedByCall bool) int {
	if extendedByCall {
		return 1
	}
	return -1
}

// Calls returns every invocation in the order it was made, each as its full
// argument slice starting with the command name.
func (f *FakeRunner) Calls() [][]string {
	return f.calls
}

func (f *FakeRunner) VerifyExecuted() {
	f.t.Helper()
	for key := range f.commands {
		if !f.executed[key] {
			f.t.Errorf("expected command was never called: %s", commandLineFromKey(key))
		}
	}
}

// RunStreaming answers exactly as Run does, and additionally writes the recorded
// output to out, so that a test can assert a command narrated itself.
func (f *FakeRunner) RunStreaming(out io.Writer, name string, args ...string) ([]byte, error) {
	output, err := f.Run(name, args...)
	if out != nil {
		_, _ = out.Write(output)
	}
	return output, err
}
