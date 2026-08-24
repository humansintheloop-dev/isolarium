//go:build e2e_ec2

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	ec2EnvironmentType    = "ec2"

	// ec2EnvironmentName is the name `--type ec2` defaults to, spelled out rather
	// than asked of the code under test, so this test pins the name a user who
	// passes no --name gets.
	ec2EnvironmentName = "isolarium-ec2"

	// agentPermissionFlag is what lets the agent edit and commit without being
	// asked: a non-interactive run has nobody to answer a permission prompt, and
	// the instance is itself the sandbox the prompt would be protecting.
	agentPermissionFlag = "--dangerously-skip-permissions"

	// agentWorkloadTimeout allows for an agent that has to read the repository
	// before it can edit it, while still failing rather than hanging out the
	// suite's own timeout when the session never comes back.
	agentWorkloadTimeout = 15 * time.Minute
)

// TestClaudeInEC2_EndToEnd drives the built binary through the whole spine of
// the capability against a real AWS account — create, an agent workload that
// leaves a commit behind, reading that commit back, destroy — so what it proves
// is the CLI a user types rather than the backend underneath it.
func TestClaudeInEC2_EndToEnd(t *testing.T) {
	environment := newEC2Environment(t)
	environment.discardWhatAnEarlierRunLeft()

	// The teardown is registered before the instance exists, because a create
	// that fails after launching one still leaves it billing.
	t.Cleanup(environment.destroyAndConfirmTermination)
	environment.create()

	commit := uniqueAgentCommit()
	environment.runAgentWorkload(commit)
	environment.assertTheAgentCommitted(commit)
}

// agentCommit is the commit the agent is asked to make and the one the next
// command reads back, held as a single value so the message cannot drift
// between the asking and the reading.
type agentCommit struct {
	message string
}

// uniqueAgentCommit carries the run's own timestamp, so that a commit left by
// anything other than this run cannot satisfy the assertion. It holds no
// spaces, so the agent has no reason to reword it and the subject read back
// needs no unquoting.
func uniqueAgentCommit() agentCommit {
	return agentCommit{message: fmt.Sprintf("e2e-ec2-%d", time.Now().UnixMilli())}
}

// prompt is quoted for the remote shell the SSH transport hands it to, which is
// why it carries its own quotes and no apostrophes.
func (c agentCommit) prompt() string {
	return fmt.Sprintf("'append the line %s to E2E.md in this repository, then commit that file with exactly this commit message: %s. do not push.'",
		c.message, c.message)
}

func (c agentCommit) isNamedBy(subject string) bool {
	return strings.Contains(subject, c.message)
}

// ec2Environment is one real instance, driven entirely through the built binary
// so that nothing the test proves depends on reaching past the CLI.
type ec2Environment struct {
	t      *testing.T
	binary string
	root   string
	name   string
}

func newEC2Environment(t *testing.T) *ec2Environment {
	t.Helper()

	built := newTestEnv(t, ec2EnvironmentType)
	return &ec2Environment{t: t, binary: built.binary, root: built.root, name: ec2EnvironmentName}
}

// discardWhatAnEarlierRunLeft hands this run the working directory a first run
// would find: create refuses to overwrite an instance declaration an abandoned
// run left behind, while destroy costs nothing when there is nothing to destroy.
func (e *ec2Environment) discardWhatAnEarlierRunLeft() {
	e.t.Helper()

	if output, err := e.runBinary("destroy", "--type", ec2EnvironmentType); err != nil {
		e.t.Fatalf("isolarium destroy --type ec2 ahead of the run: %v\n%s", err, output)
	}
}

func (e *ec2Environment) create() {
	e.t.Helper()

	args := []string{"--type", ec2EnvironmentType}
	args = append(args, envFileArgs(e.t, e.root)...)
	args = append(args, "create")
	if output, err := e.runBinary(args...); err != nil {
		e.t.Fatalf("isolarium create --type ec2: %v\n%s", err, output)
	}
}

// runAgentWorkload asks the agent for the smallest piece of real work that can
// be read back afterwards: an edit to the repository and a commit recording it.
// It goes through -i, the path a user takes, so the agent works inside the
// instance's tmux session rather than on the connection itself.
func (e *ec2Environment) runAgentWorkload(commit agentCommit) {
	e.t.Helper()

	command := exec.Command(e.binary, "run", "-i", "--type", ec2EnvironmentType, "--",
		"claude", agentPermissionFlag, "-p", commit.prompt())
	command.Dir = e.root

	terminal, err := pty.Start(command)
	if err != nil {
		e.t.Fatalf("attaching a terminal to isolarium run -i: %v", err)
	}
	defer terminal.Close()

	// The terminal is teed to stderr as it arrives, because an agent session is
	// long enough that silence would be indistinguishable from a hang.
	transcript := readPTYOutput(io.TeeReader(terminal, os.Stderr))
	e.waitForTheAgentToFinish(command, transcript)
}

func (e *ec2Environment) waitForTheAgentToFinish(command *exec.Cmd, transcript *ptyOutput) {
	e.t.Helper()

	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()

	select {
	case err := <-finished:
		if err != nil {
			e.t.Fatalf("isolarium run -i --type ec2 -- claude: %v\n%s", err, transcript.text())
		}
	case <-time.After(agentWorkloadTimeout):
		_ = command.Process.Kill()
		e.t.Fatalf("the agent had not finished %s after it was asked to work\n%s", agentWorkloadTimeout, transcript.text())
	}
}

// assertTheAgentCommitted reads the repository back through the same CLI, so
// what proves the workload ran to completion is the instance's own git history
// rather than anything the agent printed about itself.
func (e *ec2Environment) assertTheAgentCommitted(commit agentCommit) {
	e.t.Helper()

	subject := e.captureFromInstance("run", "--type", ec2EnvironmentType, "--", "git", "log", "-1", "--format=%s")
	e.t.Logf("the last commit on the instance is %q", subject)
	if !commit.isNamedBy(subject) {
		e.t.Errorf("the last commit on the instance is %q, want it to name %q — the agent was asked to make that commit",
			subject, commit.message)
	}
}

// destroyAndConfirmTermination runs as a cleanup so that a run which failed
// halfway still tears its instance down rather than leaving it billing, and it
// reports failures rather than aborting, so every step of the teardown is
// attempted.
func (e *ec2Environment) destroyAndConfirmTermination() {
	instance := e.recordedInstance()

	if output, err := e.runBinary("destroy", "--type", ec2EnvironmentType); err != nil {
		e.t.Errorf("isolarium destroy --type ec2: %v\n%s", err, output)
		return
	}
	// A create that failed before it recorded anything leaves no instance to
	// confirm the termination of, and has already failed the test on its own.
	if instance == nil {
		return
	}
	e.assertAWSHasTerminated(*instance)
}

// recordedInstance reads where create said the instance is, which has to happen
// before destroy removes the metadata holding it.
func (e *ec2Environment) recordedInstance() *ec2.Metadata {
	instance, err := ec2.NewMetadataStore(e.metadataDir(), e.name).Read()
	if err != nil {
		return nil
	}
	return instance
}

// metadataDir is the directory the CLI derives for itself from the home
// directory, resolved the same way here so the test reads the file the binary
// wrote.
func (e *ec2Environment) metadataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".isolarium")
}

// assertAWSHasTerminated asks the account itself, because the local metadata
// destroy removes would say the environment is gone whether or not anything was
// terminated.
func (e *ec2Environment) assertAWSHasTerminated(instance ec2.Metadata) {
	_, state, err := ec2.LookupInstance(context.Background(), instance.Region, instance.InstanceID)
	if err != nil {
		e.t.Errorf("asking AWS about %s after destroy: %v", instance.InstanceID, err)
		return
	}
	if !instanceState(state).isOnItsWayOut() {
		e.t.Errorf("AWS reports %s as %q after destroy, want it terminated", instance.InstanceID, state)
	}
}

// instanceState is where AWS says the instance is in its lifecycle.
type instanceState string

// isOnItsWayOut counts an instance still shutting down as terminated, because
// terraform returns once the termination is under way and AWS reports the
// settled state a moment later.
func (s instanceState) isOnItsWayOut() bool {
	return s == "terminated" || s == "shutting-down"
}

// runBinary runs one isolarium invocation from the repository root, copying its
// output to the suite's own stderr as it arrives, and hands back what it
// printed. A create that spends minutes inside terraform would otherwise say
// nothing until it exited.
func (e *ec2Environment) runBinary(args ...string) (string, error) {
	command := exec.Command(e.binary, args...)
	command.Dir = e.root

	var captured bytes.Buffer
	command.Stdout = io.MultiWriter(&captured, os.Stderr)
	command.Stderr = command.Stdout

	err := command.Run()
	return captured.String(), err
}

// captureFromInstance hands back what the instance answered, which is the last
// line of stdout rather than all of it: the CLI announces the token it mints for
// the run on the same stream before the remote command writes anything.
func (e *ec2Environment) captureFromInstance(args ...string) string {
	e.t.Helper()

	command := exec.Command(e.binary, args...)
	command.Dir = e.root
	command.Stderr = os.Stderr

	output, err := command.Output()
	if err != nil {
		e.t.Fatalf("isolarium %s: %v", strings.Join(args, " "), err)
	}
	return lastLine(string(output))
}

func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
