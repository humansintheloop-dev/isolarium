package nono

import (
	"testing"
)

func TestBuildRunCommandStartsWithNonoRun(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"})

	if cmd[0] != "nono" || cmd[1] != "run" {
		t.Errorf("expected command to start with [nono run], got %v", cmd[:2])
	}
}

func TestBuildRunCommandIncludesPermissionFlags(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"})

	assertContainsSequence(t, cmd, "--allow", ".")
	assertContainsSequence(t, cmd, "--allow", "~/.claude/")
	assertContainsSequence(t, cmd, "--allow-file", "~/.claude.json")
	assertContainsSequence(t, cmd, "--read-file", "~/Library/Keychains/login.keychain-db")
	assertContainsSequence(t, cmd, "--allow", "~/.cache/uv")
	assertContainsSequence(t, cmd, "--read", "~/.local/share/uv")
}

func TestBuildRunCommandEndsWithSeparatorAndUserArgs(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"})

	separatorIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	if separatorIdx == -1 {
		t.Fatal("expected command to contain '--' separator")
	}

	userArgs := cmd[separatorIdx+1:]
	if len(userArgs) != 2 || userArgs[0] != "echo" || userArgs[1] != "hello" {
		t.Errorf("expected user args [echo hello], got %v", userArgs)
	}
}

func TestBuildRunCommandPermissionFlagsBeforeSeparator(t *testing.T) {
	cmd := BuildRunCommand([]string{"ls"})

	separatorIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	for i := 2; i < separatorIdx; i++ {
		flag := cmd[i]
		if flag != "--allow" && flag != "--allow-file" && flag != "--read-file" && flag != "--read" &&
			flag != "." && flag != "~/.claude/" && flag != "~/.claude.json" &&
			flag != "~/.claude.json.lock" && flag != "~/.claude.json.tmp.*" &&
			flag != "~/Library/Keychains/login.keychain-db" &&
			flag != "~/.cache/uv" && flag != "~/.local/share/uv" {
			t.Errorf("unexpected flag before separator: %s", flag)
		}
	}
}
