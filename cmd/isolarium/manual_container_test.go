//go:build manual

package main

import "testing"

func TestClaudeNonInteractiveInContainer_Manual(t *testing.T) {
	output := claudeInIsolarium(t, "container")
	verifyClaudeResponded(t, output)
}
