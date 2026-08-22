//go:build ec2

// The file name is load-bearing. It sorts after repo_ec2_test.go, whose
// assertions need an instance nothing has written to, and ahead of
// tmux_ec2_test.go, whose tests deliberately leave a process running in the
// shared tmux session — which these tests need free.
package ec2_test

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	// longRunSeconds keeps the command running long enough for a second run to
	// find its session, attach to it and for a third to be refused, with margin
	// for each of those to mint a token and connect.
	longRunSeconds   = 60
	longRunExitCode  = 4
	refusedExitCode  = 5
	startedMarker    = "isolarium-run-started"
	finishedMarker   = "isolarium-run-finished"
	reattachNotice   = "reattaching to session '" + ec2.DefaultSessionName + "'"
	refusalNotice    = "already running"
	i2codeEchoedWord = "ok"

	sessionStartTimeout  = 90 * time.Second
	sessionStartInterval = 2 * time.Second
	refusalTimeout       = 90 * time.Second
	longRunTimeout       = longRunSeconds*time.Second + 3*time.Minute
)

// TestEC2Run_ReattachesToTheCommandAlreadyRunning proves, through the built
// binary, what i2code relies on: a non-interactive `run` leaves its command in
// the instance's tmux session while it runs, an identical `run` started in that
// window joins it rather than starting a second copy and ends with the same
// status, and a `run` asking for anything else is refused while naming what is
// running.
func TestEC2Run_ReattachesToTheCommandAlreadyRunning(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()

	first := environment.startIsolariumRun(longRunningCommand()...)
	environment.waitForTheSessionToBeRunning()

	reattached := environment.startIsolariumRun(longRunningCommand()...)
	refused := environment.startIsolariumRun("sh", "-c", "exit "+strconv.Itoa(refusedExitCode))

	environment.assertRunWasRefusedNamingTheRunningCommand(refused)
	environment.assertBothRunsReportTheCommandStatus(first, reattached)
	environment.assertTheReattachedRunJoinedTheSession(reattached)
	environment.assertBothRunsStreamedTheCommandOutput(first, reattached)
}

// TestEC2Run_CreateFlagRunsTheCommandWhenTheEnvironmentExists is the other half
// of the i2code invocation: the same `run --create` that created the environment
// (see TestEC2Lifecycle_Creates) finds it on the next run, skips the create, and
// runs the command — printing what it prints and exiting 0.
func TestEC2Run_CreateFlagRunsTheCommandWhenTheEnvironmentExists(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()

	exitCode, output := environment.runIsolarium(environment.runArgs(true, "echo", i2codeEchoedWord)...)

	if exitCode != 0 {
		t.Errorf("isolarium --name %s --type ec2 run --create -- echo %s exited %d, want 0\n%s", environment.name, i2codeEchoedWord, exitCode, output)
	}
	if !printsTheWord(output, i2codeEchoedWord) {
		t.Errorf("isolarium --name %s --type ec2 run --create -- echo %s did not print %q:\n%s", environment.name, i2codeEchoedWord, i2codeEchoedWord, visibleText(output))
	}
}

// longRunningCommand announces itself at both ends, so that what a run streamed
// can be told apart from what tmux paints around it, and ends with a status
// that is neither success nor the binary's own failure exit.
func longRunningCommand() []string {
	return []string{"sh", "-c", "echo " + startedMarker + "; sleep " + strconv.Itoa(longRunSeconds) + "; echo " + finishedMarker + "; exit " + strconv.Itoa(longRunExitCode)}
}

// waitForTheSessionToBeRunning is the observation the design makes: while the
// command runs, `tmux has-session -t isolarium` on the instance answers 0. It
// allows for the time the run spends minting a token and connecting.
func (e *ec2Environment) waitForTheSessionToBeRunning() {
	e.t.Helper()

	deadline := time.Now().Add(sessionStartTimeout)
	for time.Now().Before(deadline) {
		if exitCode, _ := e.askInstance("tmux", "has-session", "-t", ec2.DefaultSessionName); exitCode == 0 {
			return
		}
		time.Sleep(sessionStartInterval)
	}
	e.t.Fatalf("tmux has-session -t %s on the instance did not answer 0 within %s of starting the run", ec2.DefaultSessionName, sessionStartTimeout)
}

func (e *ec2Environment) assertRunWasRefusedNamingTheRunningCommand(refused *isolariumRun) {
	e.t.Helper()

	exitCode := refused.waitUntilItEnds(refusalTimeout)
	if exitCode == 0 || exitCode == refusedExitCode {
		e.t.Errorf("a run asking for a different command exited %d while the session was busy; want it refused rather than run\n%s", exitCode, refused.output())
	}
	running := ec2.CommandRecord(longRunningCommand())
	if !strings.Contains(refused.output(), refusalNotice) || !strings.Contains(refused.output(), running) {
		e.t.Errorf("the refused run did not name the running command %q:\n%s", running, refused.output())
	}
}

func (e *ec2Environment) assertBothRunsReportTheCommandStatus(first, reattached *isolariumRun) {
	e.t.Helper()

	if exitCode := first.waitUntilItEnds(longRunTimeout); exitCode != longRunExitCode {
		e.t.Errorf("the run that started the command exited %d, want the command's own %d\n%s", exitCode, longRunExitCode, first.output())
	}
	if exitCode := reattached.waitUntilItEnds(longRunTimeout); exitCode != longRunExitCode {
		e.t.Errorf("the run that reattached exited %d, want the command's own %d\n%s", exitCode, longRunExitCode, reattached.output())
	}
}

func (e *ec2Environment) assertTheReattachedRunJoinedTheSession(reattached *isolariumRun) {
	e.t.Helper()

	if !strings.Contains(reattached.output(), reattachNotice) {
		e.t.Errorf("the identical run did not announce %q — it may have started a second copy of the command\n%s", reattachNotice, reattached.output())
	}
}

// assertBothRunsStreamedTheCommandOutput checks both ends of the command in
// both runs: the first run streamed them as they were printed, and the
// reattached run got the start redrawn from the session on joining it and the
// end streamed live.
func (e *ec2Environment) assertBothRunsStreamedTheCommandOutput(first, reattached *isolariumRun) {
	e.t.Helper()

	for _, run := range []*isolariumRun{first, reattached} {
		for _, marker := range []string{startedMarker, finishedMarker} {
			if !strings.Contains(visibleText(run.output()), marker) {
				e.t.Errorf("%s did not stream %q:\n%s", run.label, marker, visibleText(run.output()))
			}
		}
	}
}

// isolariumRun is one invocation of the built binary in flight, started in the
// background so that a second and third can be started while it is still
// running. Its output is collected rather than streamed, because three runs
// interleaving on the suite's stderr would be unreadable; it is reported in full
// when an assertion about it fails.
type isolariumRun struct {
	t        *testing.T
	label    string
	command  *exec.Cmd
	captured *bytes.Buffer
	finished chan error
}

func (e *ec2Environment) startIsolariumRun(command ...string) *isolariumRun {
	e.t.Helper()

	run := &isolariumRun{
		t:        e.t,
		label:    "isolarium run -- " + strings.Join(command, " "),
		command:  exec.Command(isolariumBinary(e.t), e.runArgs(false, command...)...),
		captured: &bytes.Buffer{},
		finished: make(chan error, 1),
	}
	run.command.Dir = e.workDir
	run.command.Env = binaryEnvironment()
	run.command.Stdout = run.captured
	run.command.Stderr = run.captured

	if err := run.command.Start(); err != nil {
		e.t.Fatalf("starting %s: %v", run.label, err)
	}
	go func() { run.finished <- run.command.Wait() }()
	e.t.Cleanup(run.killIfStillRunning)
	return run
}

// waitUntilItEnds hands back the exit code the run reported, failing the test
// rather than hanging the suite when it has not ended in time.
func (r *isolariumRun) waitUntilItEnds(timeout time.Duration) int {
	r.t.Helper()

	select {
	case err := <-r.finished:
		exitCode, err := binaryExitCode(err)
		if err != nil {
			r.t.Fatalf("%s: %v\n%s", r.label, err, r.output())
		}
		r.finished <- nil
		return exitCode
	case <-time.After(timeout):
		r.killIfStillRunning()
		r.t.Fatalf("%s had not ended %s after it was started\n%s", r.label, timeout, r.output())
		return 0
	}
}

func (r *isolariumRun) output() string {
	return r.captured.String()
}

// killIfStillRunning makes sure a run the test gave up on cannot outlive it and
// hold the session, and the suite, open.
func (r *isolariumRun) killIfStillRunning() {
	if r.command.ProcessState != nil {
		return
	}
	_ = r.command.Process.Kill()
}

// terminalControlSequence matches what tmux writes to paint a screen — cursor
// movement, colours, the alternate-screen switch, character-set selection and
// window-title updates — which arrive in a run's output alongside the text the
// command printed.
var terminalControlSequence = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*\x07|[()][0-9A-Za-z]|[=>78@-_])`)

// nonPrintingControl matches the remaining control characters a terminal
// stream carries, leaving line breaks and tabs in place.
var nonPrintingControl = regexp.MustCompile("[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")

// visibleText is the text a run's output would show on a terminal, with the
// control sequences tmux painted it with taken out, so the command's own words
// can be looked for without an escape sequence splitting one of them.
func visibleText(output string) string {
	return nonPrintingControl.ReplaceAllString(terminalControlSequence.ReplaceAllString(output, ""), "")
}

// printsTheWord reports whether the word appears on its own in the visible
// output — not as part of another word, the way "ok" sits inside the "token"
// the binary says it is minting.
func printsTheWord(output, word string) bool {
	standalone := regexp.MustCompile(`(^|[^A-Za-z0-9])` + regexp.QuoteMeta(word) + `([^A-Za-z0-9]|$)`)
	return standalone.MatchString(visibleText(output))
}
