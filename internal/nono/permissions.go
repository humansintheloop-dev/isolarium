package nono

import (
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/git"
)

func PermissionFlags() []string {
	home := homeDir()
	flags := []string{
		"--allow", ".",
		"--allow", tempDir(),
		"--allow", "/private/tmp",
		"--allow", filepath.Join(home, ".claude") + "/",
		"--allow", filepath.Join(home, ".vscode"),
		"--allow", filepath.Join(home, "Library", "Application Support", "Code"),
		"--allow-file", filepath.Join(home, ".claude.json"),
		"--read-file", filepath.Join(home, ".gitconfig"),
		"--read", filepath.Join(home, ".config", "git"),
		"--read", filepath.Join(home, ".config", "gh"),
		"--read-file", filepath.Join(home, "Library", "Keychains", "login.keychain-db"),
		// HITL subdir name is the worktree name, but might not exist
		"--allow", filepath.Join(home, ".hitl"),
		"--read", filepath.Join(home, ".cache", "uv"),
		"--read", filepath.Join(home, ".local", "share", "uv"),
	}
	flags = append(flags, worktreeMainRepoDirFlags()...)
	return flags
}

func worktreeMainRepoDirFlags() []string {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	info, err := git.DetectWorktree(cwd)
	if err != nil || info == nil {
		return nil
	}
	return []string{"--allow", info.MainRepoDir}
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
