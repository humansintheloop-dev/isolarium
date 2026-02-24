package nono

import (
	"os"
	"path/filepath"
)

func PermissionFlags() []string {
	home := homeDir()
	return []string{
		"--allow", ".",
		"--allow", tempDir(),
		"--allow", "/private/tmp",
		"--allow", filepath.Join(home, ".claude") + "/",
		"--allow-file", filepath.Join(home, ".claude.json"),
		"--read-file", filepath.Join(home, ".gitconfig"),
		"--read-file", filepath.Join(home, "Library", "Keychains", "login.keychain-db"),
		"--allow", filepath.Join(home, ".hitl", "worktree", "logs"),
		"--allow", filepath.Join(home, ".cache", "uv"),
		"--read", filepath.Join(home, ".local", "share", "uv"),
	}
}

func tempDir() string {
	return os.TempDir()
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~"
	}
	return home
}
