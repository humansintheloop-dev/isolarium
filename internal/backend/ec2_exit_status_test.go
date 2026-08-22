package backend

import (
	"strings"
	"testing"
)

const (
	clearStatusFile = "mkdir -p ~/.isolarium && rm -f ~/.isolarium/status-isolarium"
	readStatusFile  = "cat ~/.isolarium/status-isolarium"
)

// TestEC2ExitStatus_ExecReportsTheStatusTheCommandRecorded pins that the exit
// code comes from the session's status file rather than from the tmux client,
// which exits 0 whatever the command inside it did.
func TestEC2ExitStatus_ExecReportsTheStatusTheCommandRecorded(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)
	f.instance.status = "3"

	exitCode, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"sh", "-c", "exit 3"}})

	if err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}
	if exitCode != 3 {
		t.Errorf("Exec() exit code = %d, want the 3 the command recorded", exitCode)
	}
	assertArgsEqual(t, "exec args", f.session.command.Args, sessionStart("isolarium", "sh -c 'exit 3'"))
	assertRemoteOrder(t, f.log, clearStatusFile, "tmux new-session", readStatusFile)
}

func TestEC2ExitStatus_ReattachedExecReportsTheStatusTheOriginalCommandWrote(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	f.instance.recorded = "sh -c 'exit 3'"
	f.instance.status = "3"

	exitCode, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"sh", "-c", "exit 3"}})

	if err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}
	if exitCode != 3 {
		t.Errorf("Exec() exit code = %d, want the 3 the original command recorded", exitCode)
	}
	if f.instance.asked(clearStatusFile) {
		t.Error("Exec() cleared the status file while reattaching, want the original command's status kept")
	}
	assertRemoteOrder(t, f.log, "tmux attach-session", readStatusFile)
}

func TestEC2ExitStatus_ExecClearsTheStatusFileBeforeStartingASession(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)
	f.instance.clearExit = 1

	_, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"echo", "hello"}})

	if err == nil {
		t.Fatal("Exec() returned nil error when the stale status file could not be cleared")
	}
	if !f.instance.asked(clearStatusFile) {
		t.Error("Exec() never asked the instance to clear the status file")
	}
	if f.session.called {
		t.Errorf("Exec() started the session with %v despite the uncleared status file", f.session.command.Args)
	}
	if !strings.Contains(err.Error(), "~/.isolarium/status-isolarium") {
		t.Errorf("Exec() error = %q, want it to name the status file", err)
	}
}

func TestEC2ExitStatus_ExecFailsWhenNoStatusWasRecorded(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)
	f.instance.status = ""

	exitCode, err := f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"echo", "hello"}})

	if err == nil {
		t.Fatal("Exec() returned nil error with no status file, want the missing status reported rather than success")
	}
	if exitCode != 1 {
		t.Errorf("Exec() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(err.Error(), "~/.isolarium/status-isolarium") {
		t.Errorf("Exec() error = %q, want it to name the status file", err)
	}
	if !f.session.called {
		t.Error("Exec() never started the session, want the missing status noticed only after the client returned")
	}
}

// assertRemoteOrder checks that each marker was sent to the instance, and that
// they were sent in the order given.
func assertRemoteOrder(t *testing.T, log *ec2RemoteLog, markers ...string) {
	t.Helper()

	next := 0
	for _, marker := range markers {
		index := indexOfLineWith(log.lines[next:], marker)
		if index < 0 {
			t.Fatalf("the instance was never asked %q after position %d; it was asked, in order:\n%s", marker, next, strings.Join(log.lines, "\n"))
		}
		next += index + 1
	}
}

func indexOfLineWith(lines []string, marker string) int {
	for i, line := range lines {
		if strings.Contains(line, marker) {
			return i
		}
	}
	return -1
}
