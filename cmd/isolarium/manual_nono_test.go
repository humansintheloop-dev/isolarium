//go:build manual

package main

import "testing"

func TestClaudeNonInteractiveInNono_Manual(t *testing.T) {
	output := claudeInIsolarium(t, "nono")
	verifyClaudeResponded(t, output)
}
