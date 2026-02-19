package nono

import (
	"path/filepath"
	"testing"
)

func TestPermissionFlagsContainsProjectDirectoryAccess(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", ".")
}

func TestPermissionFlagsContainsClaudeConfigDirectory(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", filepath.Join(homeDir(), ".claude")+"/")
}

func TestPermissionFlagsContainsClaudeSettingsFile(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow-file", filepath.Join(homeDir(), ".claude.json"))
}

func TestPermissionFlagsContainsMacOSKeychainReadOnly(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--read-file", filepath.Join(homeDir(), "Library", "Keychains", "login.keychain-db"))
}

func TestPermissionFlagsContainsUvCache(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--allow", filepath.Join(homeDir(), ".cache", "uv"))
}

func TestPermissionFlagsContainsUvDataReadOnly(t *testing.T) {
	flags := PermissionFlags()

	assertContainsSequence(t, flags, "--read", filepath.Join(homeDir(), ".local", "share", "uv"))
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
