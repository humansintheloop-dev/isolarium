//go:build e2e_claude

package main

import "testing"

func TestClaudeNonInteractiveInContainer_EndToEnd(t *testing.T) {
	env := newTestEnv(t, "container")
	env.ensureReady()
	verifyClaudeResponded(t, env.runClaude())
}
