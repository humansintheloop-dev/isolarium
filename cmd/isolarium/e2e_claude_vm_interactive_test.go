//go:build e2e_claude

package main

import "testing"

func TestClaudeInteractiveInVM_EndToEnd(t *testing.T) {
	claudeInteractiveInIsolarium(t, "vm")
}
