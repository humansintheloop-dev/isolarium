//go:build ec2

package ec2_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	writerScriptPath      = instanceHomeDir + "/writer.sh"
	writerLogPath         = instanceHomeDir + "/writer.log"
	writerPIDPath         = instanceHomeDir + "/writer.pid"
	writeInterval         = time.Second
	intervalsSpentOffline = 5
	writerStartTimeout    = 60 * time.Second
	writerStartInterval   = time.Second
	sshChildTimeout       = 60 * time.Second
	sshChildInterval      = 200 * time.Millisecond
	bothSessionsTimeout   = 60 * time.Second
	bothSessionsInterval  = 2 * time.Second
	sessionCloseTimeout   = 60 * time.Second
)

// additionalSessionName is the session --new-session has to open while
// `isolarium` is the only one running. It is spelled out rather than asked of
// the code under test, so the test pins the name the user is given.
const additionalSessionName = ec2.DefaultSessionName + "-2"

// interactiveSSHPattern is the part of the local ssh command line that only the
// connection carrying an interactive command has.
const interactiveSSHPattern = "tmux new-session"

// writerScript is the long-running process the disconnect has to leave alive. It
// records its own PID so the reattached connection can prove it is looking at
// the same process rather than a second one started behind its back. The PID
// file is written last, so its arrival means the log is there to be read too.
const writerScript = `#!/bin/bash
: > ` + writerLogPath + `
echo $$ > ` + writerPIDPath + `
while true; do
  date +%s >> ` + writerLogPath + `
  sleep 1
done
`

// TestEC2Session_SurvivesDisconnect proves the capability the EC2 backend exists
// for: work started on the instance outlives the connection that started it, so
// a closed laptop or a dropped network costs nothing.
func TestEC2Session_SurvivesDisconnect(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()
	attachPseudoTerminal(t)
	environment.placeWriterScript()

	connection := environment.startWriterInsideTmuxSession()
	beforeDisconnect := environment.waitForWriterToStart()
	environment.assertOnlyTheIsolariumSessionIsRunning()

	connection.killAbruptly()
	sleepPastSeveralWriteIntervals()

	reattached := environment.reattachToTmuxSession()
	afterReattach := environment.observeWriter()

	environment.assertTheSameProcessIsStillWriting(beforeDisconnect, afterReattach)
	environment.assertOnlyTheIsolariumSessionIsRunning()
	reattached.killAbruptly()
}

// TestEC2Session_NewSessionLeavesExistingUntouched proves --new-session adds a
// session rather than taking over the one an agent is already working in: the
// two run side by side, and once the second has closed the first is still
// carrying the same process it was given.
func TestEC2Session_NewSessionLeavesExistingUntouched(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()
	attachPseudoTerminal(t)
	environment.placeWriterScript()

	firstConnection := environment.startWriterInsideTmuxSession()
	beforeTheAdditionalSession := environment.waitForWriterToStart()

	additional := environment.startAdditionalSession()
	environment.waitForBothSessionsToRunSideBySide()
	environment.closeAdditionalSession(additional)

	afterTheAdditionalSession := environment.observeWriter()
	environment.assertTheSameProcessIsStillWriting(beforeTheAdditionalSession, afterTheAdditionalSession)
	environment.assertOnlyTheIsolariumSessionIsRunning()
	firstConnection.killAbruptly()
}

// clearWhatAnEarlierTmuxTestLeftBehind gives this test the instance in the state
// a first tmux test would find it in. Both things it removes matter: TmuxCommand
// runs `tmux new-session -A`, which would attach to the session the previous
// test deliberately left running rather than start the writer at all, and the
// writer's own PID file would otherwise still be there from that run, so waiting
// for the writer to start would read the dead process's PID.
func (e *ec2Environment) clearWhatAnEarlierTmuxTestLeftBehind() {
	e.t.Helper()

	e.askInstance("tmux", "kill-server")
	e.askInstance("rm", "-f", writerPIDPath, writerLogPath)
}

// startAdditionalSession opens a second session the way --new-session does. Its
// command blocks forever, so the session stays up for as long as the test needs
// both sessions running and ends only when the test closes it.
func (e *ec2Environment) startAdditionalSession() *additionalSession {
	e.t.Helper()

	e.rejoinTheSharedSessionOnceTheTestIsOver()
	e.backend.UseNewSession()
	return &additionalSession{t: e.t, finished: e.launchInteractive("tail", "-f", "/dev/null")}
}

// rejoinTheSharedSessionOnceTheTestIsOver undoes UseNewSession, because the
// backend is shared with every later test on this instance and they expect the
// ordinary behaviour of joining the one session.
func (e *ec2Environment) rejoinTheSharedSessionOnceTheTestIsOver() {
	e.t.Cleanup(func() { e.backend.SessionNameFunc = nil })
}

// additionalSession is the second concurrent session in flight. It is kept apart
// from remoteConnection because nothing severs it: it is closed and waited out.
type additionalSession struct {
	t        *testing.T
	finished chan int
}

// closeAdditionalSession ends the second session from the instance, the way a
// user finishing with it would, and waits for the connection carrying it to go.
// Killing it by name would fail were --new-session to have joined the session
// the writer is in rather than opening its own.
func (e *ec2Environment) closeAdditionalSession(additional *additionalSession) {
	e.t.Helper()

	e.instanceOutput("tmux", "kill-session", "-t", additionalSessionName)
	additional.waitUntilItCloses()
}

func (s *additionalSession) waitUntilItCloses() {
	s.t.Helper()

	select {
	case <-s.finished:
	case <-time.After(sessionCloseTimeout):
		s.t.Fatalf("the connection to %s was still open %s after the session was killed", additionalSessionName, sessionCloseTimeout)
	}
}

// waitForBothSessionsToRunSideBySide allows for the time the second connection
// spends reaching the instance, so a session still being opened is not mistaken
// for one --new-session never created.
func (e *ec2Environment) waitForBothSessionsToRunSideBySide() {
	e.t.Helper()

	want := []string{ec2.DefaultSessionName, additionalSessionName}
	var running []string
	deadline := time.Now().Add(bothSessionsTimeout)
	for time.Now().Before(deadline) {
		running = e.runningSessionNames()
		if slices.Equal(running, want) {
			return
		}
		time.Sleep(bothSessionsInterval)
	}
	e.t.Fatalf("tmux list-sessions reports %v after %s, want %v running side by side", running, bothSessionsTimeout, want)
}

// attachPseudoTerminal gives the test process a terminal on stdin and stdout for
// the length of the test, because `ssh -t` declines to allocate a remote pty when
// the local end is a pipe, and tmux then refuses to start at all.
func attachPseudoTerminal(t *testing.T) {
	t.Helper()

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	discardTerminalOutput(master)

	originalStdin, originalStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = slave, slave
	t.Cleanup(func() {
		os.Stdin, os.Stdout = originalStdin, originalStdout
		_ = slave.Close()
		_ = master.Close()
	})
}

// discardTerminalOutput keeps reading what tmux paints, so the pseudo-terminal's
// buffer never fills and blocks the connection under test.
func discardTerminalOutput(master *os.File) {
	go func() { _, _ = io.Copy(io.Discard, master) }()
}

func (e *ec2Environment) placeWriterScript() {
	e.t.Helper()

	localPath := filepath.Join(e.t.TempDir(), "writer.sh")
	if err := os.WriteFile(localPath, []byte(writerScript), 0o755); err != nil {
		e.t.Fatalf("writing the writer script to %s: %v", localPath, err)
	}

	session := ec2.NewInstanceSession(e.base, e.publicDNS, ec2.ExecCommand)
	if err := ec2.CopyFileToInstance(session, localPath, writerScriptPath); err != nil {
		e.t.Fatalf("placing %s on the instance: %v", writerScriptPath, err)
	}
	e.instanceOutput("chmod", "+x", writerScriptPath)
}

func (e *ec2Environment) startWriterInsideTmuxSession() *remoteConnection {
	e.t.Helper()

	return e.execInteractive(writerScriptPath)
}

// reattachToTmuxSession joins the session the killed connection left behind.
// `tmux new-session -A` discards the command it is handed once the session
// exists, so what this second connection asks to run is deliberately inert.
func (e *ec2Environment) reattachToTmuxSession() *remoteConnection {
	e.t.Helper()

	return e.execInteractive("true")
}

// remoteConnection is one ExecInteractive in flight: the local ssh process
// carrying it, and the exit code it reports once that process is gone.
type remoteConnection struct {
	t        *testing.T
	sshPID   int
	finished chan int
}

func (e *ec2Environment) execInteractive(args ...string) *remoteConnection {
	e.t.Helper()

	finished := e.launchInteractive(args...)
	return &remoteConnection{t: e.t, sshPID: waitForInteractiveSSHProcess(e.t), finished: finished}
}

// launchInteractive starts the connection and hands back the exit code it will
// eventually report, leaving the caller to decide how the connection ends.
func (e *ec2Environment) launchInteractive(args ...string) chan int {
	e.t.Helper()

	finished := make(chan int, 1)
	go func() {
		exitCode, err := e.backend.ExecInteractive(backend.ExecRequest{ContainerName: e.name, Args: args})
		if err != nil {
			e.t.Errorf("ExecInteractive of %q: %v", strings.Join(args, " "), err)
		}
		finished <- exitCode
	}()
	return finished
}

// killAbruptly severs the connection the way a closed laptop or a dropped
// network does — the local ssh process dies without telling the instance —
// rather than the way a clean exit does.
func (c *remoteConnection) killAbruptly() {
	c.t.Helper()

	if err := syscall.Kill(c.sshPID, syscall.SIGKILL); err != nil {
		c.t.Fatalf("SIGKILLing the local ssh process %d: %v", c.sshPID, err)
	}
	<-c.finished
}

// waitForInteractiveSSHProcess finds the ssh process ExecInteractive spawned,
// looking only at children of this test process so no unrelated ssh on the
// machine can be mistaken for it.
func waitForInteractiveSSHProcess(t *testing.T) int {
	t.Helper()

	deadline := time.Now().Add(sshChildTimeout)
	for time.Now().Before(deadline) {
		if pid, found := interactiveSSHChildProcess(); found {
			return pid
		}
		time.Sleep(sshChildInterval)
	}
	t.Fatalf("ExecInteractive did not spawn an ssh process within %s", sshChildTimeout)
	return 0
}

// interactiveSSHChildProcess matches on the tmux wrapper rather than on ssh
// itself, because ExecInteractive first sends a short-lived `tmux has-session`
// probe over its own ssh — and killing that probe would sever nothing.
func interactiveSSHChildProcess() (int, bool) {
	output, err := exec.Command("pgrep", "-P", strconv.Itoa(os.Getpid()), "-f", interactiveSSHPattern).Output()
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(firstLine(string(output)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func sleepPastSeveralWriteIntervals() {
	time.Sleep(intervalsSpentOffline * writeInterval)
}

// writerState is what the instance says about the long-running process at one
// point in time: which process is doing the writing, and how much it has
// written.
type writerState struct {
	pid   string
	lines int
}

func (e *ec2Environment) waitForWriterToStart() writerState {
	e.t.Helper()

	deadline := time.Now().Add(writerStartTimeout)
	for time.Now().Before(deadline) {
		if exitCode, _ := e.askInstance("test", "-s", writerPIDPath); exitCode == 0 {
			return e.observeWriter()
		}
		time.Sleep(writerStartInterval)
	}
	e.t.Fatalf("the writer did not start inside the tmux session within %s", writerStartTimeout)
	return writerState{}
}

func (e *ec2Environment) observeWriter() writerState {
	e.t.Helper()

	return writerState{
		pid:   e.instanceOutput("cat", writerPIDPath),
		lines: e.countWriterLogLines(),
	}
}

func (e *ec2Environment) countWriterLogLines() int {
	e.t.Helper()

	reported := e.instanceOutput("wc", "-l", writerLogPath)
	fields := strings.Fields(reported)
	if len(fields) == 0 {
		e.t.Fatalf("wc -l %s reported %q, want a line count", writerLogPath, reported)
	}
	count, err := strconv.Atoi(fields[0])
	if err != nil {
		e.t.Fatalf("wc -l %s reported %q, want a line count: %v", writerLogPath, reported, err)
	}
	return count
}

func (e *ec2Environment) assertTheSameProcessIsStillWriting(before, after writerState) {
	e.t.Helper()

	if after.pid != before.pid {
		e.t.Errorf("the writer's PID is %q after reattaching, want %q — the connection started a second process instead of rejoining the first",
			after.pid, before.pid)
	}
	if after.lines <= before.lines {
		e.t.Errorf("%s holds %d lines after reattaching, want more than the %d it held before the disconnect",
			writerLogPath, after.lines, before.lines)
	}
	e.assertProcessIsStillTheWriter(after)
}

// assertProcessIsStillTheWriter reads back what the recorded PID is running, so
// a PID the kernel handed to something else cannot pass for a writer that died
// with the connection.
func (e *ec2Environment) assertProcessIsStillTheWriter(state writerState) {
	e.t.Helper()

	exitCode, output := e.askInstance("ps", "-p", state.pid, "-o", "args=")
	if exitCode != 0 {
		e.t.Fatalf("process %s is no longer running on the instance", state.pid)
	}
	if !strings.Contains(output, writerScriptPath) {
		e.t.Errorf("process %s is running %q, want %s", state.pid, strings.TrimSpace(output), writerScriptPath)
	}
}

func (e *ec2Environment) assertOnlyTheIsolariumSessionIsRunning() {
	e.t.Helper()

	if names := e.runningSessionNames(); !slices.Equal(names, []string{ec2.DefaultSessionName}) {
		e.t.Errorf("tmux list-sessions reports %v, want exactly one session named %q", names, ec2.DefaultSessionName)
	}
}

// runningSessionNames sorts what the instance reports, so comparing it against
// an expected set does not depend on the order tmux happens to list sessions in.
func (e *ec2Environment) runningSessionNames() []string {
	e.t.Helper()

	var names []string
	for _, line := range strings.Split(e.instanceOutput("tmux", "list-sessions"), "\n") {
		if name, _, found := strings.Cut(line, ":"); found {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func (e *ec2Environment) instanceOutput(args ...string) string {
	e.t.Helper()

	exitCode, output := e.askInstance(args...)
	if exitCode != 0 {
		e.t.Fatalf("%s on the instance exited %d, want 0; output: %s", strings.Join(args, " "), exitCode, output)
	}
	return strings.TrimSpace(output)
}

// askInstance runs args on the instance and hands back its exit status together
// with what it wrote. It collects that output into a buffer of its own rather
// than by borrowing the process-wide stdout, because this test reads the
// instance while interactive connections are being opened — and a connection
// that started mid-read would inherit the borrowed stdout in place of its
// terminal and hold it open for as long as the session lived.
func (e *ec2Environment) askInstance(args ...string) (int, string) {
	e.t.Helper()

	output, exitCode, err := ec2.CaptureCommand(e.base, e.publicDNS, ec2.RemoteCommand{Args: args})
	if err != nil {
		e.t.Fatalf("running %s on the instance: %v", strings.Join(args, " "), err)
	}
	return exitCode, output
}
