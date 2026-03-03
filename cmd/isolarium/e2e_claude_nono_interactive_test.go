//go:build e2e_claude

package main

import "testing"

func TestClaudeInteractiveInNono_EndToEnd(t *testing.T) {
	claudeInteractiveInIsolarium(t, "nono")
}
