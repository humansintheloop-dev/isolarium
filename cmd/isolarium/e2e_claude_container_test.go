//go:build e2e_claude

package main

import "testing"

func TestClaudeNonInteractiveInContainer_EndToEnd(t *testing.T) {
	output := claudeInIsolarium(t, "container")
	verifyClaudeResponded(t, output)
}
