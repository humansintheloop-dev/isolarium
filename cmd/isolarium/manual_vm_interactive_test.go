//go:build manual

package main

import "testing"

func TestClaudeInteractiveInVM_Manual(t *testing.T) {
	claudeInteractiveInIsolarium(t, "vm")
}
