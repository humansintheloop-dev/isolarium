//go:build ec2 && ec2_claude

package ec2_test

import (
	"os"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/claude"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	hostCredentialsPathEnvVar = "CLAUDE_CREDENTIALS_PATH"
	ownerOnlyFileMode         = "600"
	authenticationPrompt      = "'reply with the single word ok'"
	expectedReply             = "ok"
)

// TestEC2Instance_ClaudeAuthenticates proves that the credentials isolarium
// copies to the instance are the ones claude authenticates with there, rather
// than only that the copy issued the commands it was asked to.
//
// It never rewinds claudeAiOauth.expiresAt and asserts nothing about token
// refresh: whether Claude Code refreshes its own token on Linux is assumption
// A1, accepted unverified for v1.
func TestEC2Instance_ClaudeAuthenticates(t *testing.T) {
	credentials := requireHostClaudeCredentials(t)
	environment := sharedInstance(t)

	environment.copyClaudeCredentials(credentials)

	environment.assertCredentialsAreReadableOnlyByTheirOwner()
	environment.assertClaudeAnswersThePrompt()
}

// requireHostClaudeCredentials fails the run rather than skipping it when the
// host has no Claude credentials to copy, because a skipped test would report
// green while proving nothing about authentication.
func requireHostClaudeCredentials(t *testing.T) string {
	t.Helper()

	credentials, err := claude.ReadCredentialsFromKeychain()
	if err == nil {
		return credentials
	}
	t.Logf("reading the Claude keychain entry failed (%v); falling back to %s", err, hostCredentialsPathEnvVar)
	return readCredentialsFile(t, requireEnvVar(t, hostCredentialsPathEnvVar))
}

func readCredentialsFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the Claude credentials from %s: %v", path, err)
	}
	return string(data)
}

// copyClaudeCredentials carries the host blob to the instance exactly as
// run --copy-session does.
func (e *ec2Environment) copyClaudeCredentials(credentials string) {
	e.t.Helper()

	if err := e.backend.CopyCredentials(e.name, credentials); err != nil {
		e.t.Fatalf("copying Claude credentials to %s: %v", e.name, err)
	}
}

func (e *ec2Environment) assertCredentialsAreReadableOnlyByTheirOwner() {
	e.t.Helper()

	exitCode, output := e.askInstance("stat", "-c", "%a", ec2.RemoteCredentialsPath)
	if exitCode != 0 {
		e.t.Fatalf("stat of %s exited %d, want 0; output: %s", ec2.RemoteCredentialsPath, exitCode, output)
	}
	if strings.TrimSpace(output) != ownerOnlyFileMode {
		e.t.Errorf("%s mode = %s, want %s — a refresh token on a public-facing host must not be readable by anyone else",
			ec2.RemoteCredentialsPath, strings.TrimSpace(output), ownerOnlyFileMode)
	}
}

func (e *ec2Environment) assertClaudeAnswersThePrompt() {
	e.t.Helper()

	exitCode, output := e.askInstance("claude", "-p", authenticationPrompt)
	if exitCode != 0 {
		e.t.Fatalf("claude -p %s exited %d, want 0; output: %s", authenticationPrompt, exitCode, output)
	}
	if !isTheExpectedReply(output) {
		e.t.Errorf("claude -p %s answered %q, want %q", authenticationPrompt, strings.TrimSpace(output), expectedReply)
	}
}

// isTheExpectedReply tolerates the punctuation and capitalisation a language
// model may wrap a one-word answer in, while still rejecting any other answer.
func isTheExpectedReply(output string) bool {
	answer := strings.Trim(strings.TrimSpace(output), ".!\"'")
	return strings.EqualFold(answer, expectedReply)
}
