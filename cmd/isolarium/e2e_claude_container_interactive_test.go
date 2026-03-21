//go:build e2e_claude

package main

import "testing"

func TestClaudeInteractiveInContainer_EndToEnd(t *testing.T) {
	env := newTestEnv(t, "container")
	env.ensureReady()
	env.runClaudeInteractive()
}
