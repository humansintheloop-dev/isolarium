//go:build manual

package main

import "testing"

func TestClaudeInteractiveInContainer_Manual(t *testing.T) {
	claudeInteractiveInIsolarium(t, "container")
}
