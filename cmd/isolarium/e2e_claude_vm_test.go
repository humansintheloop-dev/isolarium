//go:build e2e_claude

package main

import "testing"

func TestClaudeNonInteractiveInVM_EndToEnd(t *testing.T) {
	env := newTestEnv(t, "vm")
	env.ensureReady()
	verifyClaudeResponded(t, env.runClaude())
}
