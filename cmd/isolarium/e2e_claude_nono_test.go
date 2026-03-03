//go:build e2e_claude

package main

import "testing"

func TestClaudeNonInteractiveInNono_EndToEnd(t *testing.T) {
	output := claudeInIsolarium(t, "nono")
	verifyClaudeResponded(t, output)
}
