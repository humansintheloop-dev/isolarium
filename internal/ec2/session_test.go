package ec2

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// readCredentialsLine is the one command a copy that decides to leave the
// instance alone is allowed to issue.
const readCredentialsLine = "cat ~/.claude/.credentials.json 2>/dev/null"

// credentialExec stands in for the SSH transport: it records every command the
// copy issues and answers the read with a canned instance-side credential file.
type credentialExec struct {
	readOutput    string
	readExitCode  int
	writeExitCode int
	base          string
	publicDNS     string
	commands      []RemoteCommand
}

func (e *credentialExec) run(base, publicDNS string, cmd RemoteCommand) (string, int, error) {
	e.base = base
	e.publicDNS = publicDNS
	e.commands = append(e.commands, cmd)
	if isCredentialRead(cmd) {
		return e.readOutput, e.readExitCode, nil
	}
	return "", e.writeExitCode, nil
}

func isCredentialRead(cmd RemoteCommand) bool {
	return strings.Join(cmd.Args, " ") == readCredentialsLine
}

func (e *credentialExec) commandLines() []string {
	lines := make([]string, 0, len(e.commands))
	for _, cmd := range e.commands {
		lines = append(lines, strings.Join(cmd.Args, " "))
	}
	return lines
}

func credentialsExpiring(expiresAt int64) string {
	return fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok-%d","expiresAt":%d}}`, expiresAt, expiresAt)
}

func copyCredentials(t *testing.T, exec *credentialExec, credentials string) {
	t.Helper()

	if err := NewInstanceQuery(t.TempDir(), testPublicDNS, exec.run).CopyClaudeCredentials(credentials); err != nil {
		t.Fatalf("CopyClaudeCredentials() error = %v, want nil", err)
	}
}

func assertLeftInstanceUntouched(t *testing.T, exec *credentialExec) {
	t.Helper()

	assertCommandEquals(t, exec.commandLines(), []string{readCredentialsLine})
}

func assertWroteCredentials(t *testing.T, exec *credentialExec, credentials string) {
	t.Helper()

	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
	assertCommandEquals(t, exec.commandLines(), []string{
		readCredentialsLine,
		"mkdir -p ~/.claude",
		"bash -c 'echo " + encoded + " | base64 -d > ~/.claude/.credentials.json'",
		"chmod 600 ~/.claude/.credentials.json",
	})
}

func TestCopyClaudeCredentials_SkipsWhenInstanceIsNewer(t *testing.T) {
	exec := &credentialExec{readOutput: credentialsExpiring(2000)}

	copyCredentials(t, exec, credentialsExpiring(1000))

	assertLeftInstanceUntouched(t, exec)
}

func TestCopyClaudeCredentials_SkipsWhenEqual(t *testing.T) {
	exec := &credentialExec{readOutput: credentialsExpiring(1000)}

	copyCredentials(t, exec, credentialsExpiring(1000))

	assertLeftInstanceUntouched(t, exec)
}

func TestCopyClaudeCredentials_WritesWhenHostIsNewer(t *testing.T) {
	exec := &credentialExec{readOutput: credentialsExpiring(1000)}
	hostCredentials := credentialsExpiring(2000)

	copyCredentials(t, exec, hostCredentials)

	assertWroteCredentials(t, exec, hostCredentials)
}

func TestCopyClaudeCredentials_WritesWhenInstanceFileAbsent(t *testing.T) {
	exec := &credentialExec{readExitCode: 1}
	hostCredentials := credentialsExpiring(1000)

	copyCredentials(t, exec, hostCredentials)

	assertWroteCredentials(t, exec, hostCredentials)
}

func TestCopyClaudeCredentials_WritesWhenInstanceFileUnparseable(t *testing.T) {
	exec := &credentialExec{readOutput: "not json at all"}
	hostCredentials := credentialsExpiring(1000)

	copyCredentials(t, exec, hostCredentials)

	assertWroteCredentials(t, exec, hostCredentials)
}

func TestCopyClaudeCredentials_WritesWhenInstanceExpiresAtMissing(t *testing.T) {
	exec := &credentialExec{readOutput: `{"claudeAiOauth":{"accessToken":"tok"}}`}
	hostCredentials := credentialsExpiring(1000)

	copyCredentials(t, exec, hostCredentials)

	assertWroteCredentials(t, exec, hostCredentials)
}

// TestCopyClaudeCredentials_ReachesTheInstanceItWasGiven pins the transport
// arguments down, because a copy that wrote a credential file onto the wrong
// host would otherwise satisfy every other assertion here.
func TestCopyClaudeCredentials_ReachesTheInstanceItWasGiven(t *testing.T) {
	base := t.TempDir()
	exec := &credentialExec{readExitCode: 1}

	if err := NewInstanceQuery(base, testPublicDNS, exec.run).CopyClaudeCredentials(credentialsExpiring(1000)); err != nil {
		t.Fatalf("CopyClaudeCredentials() error = %v, want nil", err)
	}

	if exec.base != base {
		t.Errorf("CopyClaudeCredentials() used base %q, want %q", exec.base, base)
	}
	if exec.publicDNS != testPublicDNS {
		t.Errorf("CopyClaudeCredentials() used public DNS %q, want %q", exec.publicDNS, testPublicDNS)
	}
}

func TestCopyClaudeCredentials_ReportsARejectedWrite(t *testing.T) {
	exec := &credentialExec{readExitCode: 1, writeExitCode: 1}

	err := NewInstanceQuery(t.TempDir(), testPublicDNS, exec.run).CopyClaudeCredentials(credentialsExpiring(1000))

	if err == nil {
		t.Fatal("CopyClaudeCredentials() returned nil error when the instance rejected the write")
	}
}

func TestReadInstanceExpiresAt_ReportsTheInstancesOwnExpiry(t *testing.T) {
	exec := &credentialExec{readOutput: credentialsExpiring(1700)}

	expiresAt, found := NewInstanceQuery(t.TempDir(), testPublicDNS, exec.run).ReadInstanceExpiresAt()

	if !found {
		t.Fatal("ReadInstanceExpiresAt() found no credentials on an instance that has them")
	}
	if expiresAt != 1700 {
		t.Errorf("ReadInstanceExpiresAt() = %d, want 1700", expiresAt)
	}
}

func TestReadInstanceExpiresAt_ReportsNoAnswerForAnUnreadableFile(t *testing.T) {
	exec := &credentialExec{readExitCode: 1}

	if _, found := NewInstanceQuery(t.TempDir(), testPublicDNS, exec.run).ReadInstanceExpiresAt(); found {
		t.Error("ReadInstanceExpiresAt() reported an answer for a file the instance could not read")
	}
}
