//go:build e2e_claude

package main

import "testing"

func TestClaudeInteractiveInContainer_EndToEnd(t *testing.T) {
	claudeInteractiveInIsolarium(t, "container")
}
