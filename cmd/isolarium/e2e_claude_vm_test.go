//go:build e2e_claude

package main

import "testing"

func TestClaudeNonInteractiveInVM_EndToEnd(t *testing.T) {
	output := claudeInIsolarium(t, "vm")
	verifyClaudeResponded(t, output)
}
