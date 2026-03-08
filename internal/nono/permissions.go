package nono

import (
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/git"
)

func PermissionFlags() []string {
	flags := []string{
		"--allow-cwd",
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
