//go:build manual

package main

import "testing"

func TestClaudeNonInteractiveInVM_Manual(t *testing.T) {
	output := claudeInIsolarium(t, "vm")
	verifyClaudeResponded(t, output)
}
