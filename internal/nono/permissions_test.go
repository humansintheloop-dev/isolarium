package nono

import (
	"testing"
)

func TestPermissionFlagsContainsProjectDirectoryAccess(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", ".")
}

func TestPermissionFlagsContainsClaudeConfigDirectory(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", "~/.claude/")
}

func TestPermissionFlagsContainsClaudeSettingsFile(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow-file", "~/.claude.json")
}

func TestPermissionFlagsContainsClaudeLockFile(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", "~/.claude.json.lock")
}

func TestPermissionFlagsContainsClaudeTempFiles(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", "~/.claude.json.tmp.*")
}

func TestPermissionFlagsContainsMacOSKeychainReadOnly(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--read-file", "~/Library/Keychains/login.keychain-db")
}

func TestPermissionFlagsContainsUvCache(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", "~/.cache/uv")
}

func TestPermissionFlagsContainsUvDataReadOnly(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--read", "~/.local/share/uv")
}

func assertContainsSequence(t *testing.T, slice []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(slice)-1; i++ {
		if slice[i] == flag && slice[i+1] == value {
			return
		}
	}
	t.Errorf("expected flags to contain [%s %s], got %v", flag, value, slice)
}
