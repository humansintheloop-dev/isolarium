//go:build e2e_claude

package main

import "testing"

func TestClaudeNonInteractiveInNono_EndToEnd(t *testing.T) {
	env := newTestEnv(t, "nono")
	env.ensureReady()
	verifyClaudeResponded(t, env.runClaude())
}
