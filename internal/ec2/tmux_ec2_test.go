//go:build ec2

package ec2_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	tmuxEnvironmentName   = "isolarium-ec2-tmux-test"
	writerScriptPath      = instanceHomeDir + "/writer.sh"
	writerLogPath         = instanceHomeDir + "/writer.log"
	writerPIDPath         = instanceHomeDir + "/writer.pid"
	writeInterval         = time.Second
	intervalsSpentOffline = 5
	writerStartTimeout    = 60 * time.Second
	writerStartInterval   = time.Second
	sshChildTimeout       = 60 * time.Second
	sshChildInterval      = 200 * time.Millisecond
)

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
	environment := startEC2Environment(t, tmuxEnvironmentName)
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

	finished := make(chan int, 1)
	go func() {
		exitCode, err := e.backend.ExecInteractive(backend.ExecRequest{ContainerName: e.name, Args: args})
		if err != nil {
			e.t.Errorf("ExecInteractive of %q: %v", strings.Join(args, " "), err)
		}
		finished <- exitCode
	}()
	return &remoteConnection{t: e.t, sshPID: waitForInteractiveSSHProcess(e.t), finished: finished}
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
		if exitCode, _ := e.run("test", "-s", writerPIDPath); exitCode == 0 {
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
		lines: e.countLines(writerLogPath),
	}
}

func (e *ec2Environment) countLines(remotePath string) int {
	e.t.Helper()

	reported := e.instanceOutput("wc", "-l", remotePath)
	fields := strings.Fields(reported)
	if len(fields) == 0 {
		e.t.Fatalf("wc -l %s reported %q, want a line count", remotePath, reported)
	}
	count, err := strconv.Atoi(fields[0])
	if err != nil {
		e.t.Fatalf("wc -l %s reported %q, want a line count: %v", remotePath, reported, err)
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
	e.assertProcessIsStillTheWriter(after.pid)
}

// assertProcessIsStillTheWriter reads back what the recorded PID is running, so
// a PID the kernel handed to something else cannot pass for a writer that died
// with the connection.
func (e *ec2Environment) assertProcessIsStillTheWriter(pid string) {
	e.t.Helper()

	exitCode, output := e.run("ps", "-p", pid, "-o", "args=")
	if exitCode != 0 {
		e.t.Fatalf("process %s is no longer running on the instance", pid)
	}
	if !strings.Contains(output, writerScriptPath) {
		e.t.Errorf("process %s is running %q, want %s", pid, strings.TrimSpace(output), writerScriptPath)
	}
}

func (e *ec2Environment) assertOnlyTheIsolariumSessionIsRunning() {
	e.t.Helper()

	names := e.listedSessionNames()
	if len(names) != 1 || names[0] != ec2.DefaultSessionName {
		e.t.Errorf("tmux list-sessions reports %v, want exactly one session named %q", names, ec2.DefaultSessionName)
	}
}

func (e *ec2Environment) listedSessionNames() []string {
	e.t.Helper()

	var names []string
	for _, line := range strings.Split(e.instanceOutput("tmux", "list-sessions"), "\n") {
		if name, _, found := strings.Cut(line, ":"); found {
			names = append(names, name)
		}
	}
	return names
}

func (e *ec2Environment) instanceOutput(args ...string) string {
	e.t.Helper()

	exitCode, output := e.run(args...)
	if exitCode != 0 {
		e.t.Fatalf("%s on the instance exited %d, want 0; output: %s", strings.Join(args, " "), exitCode, output)
	}
	return strings.TrimSpace(output)
}
