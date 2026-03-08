package nono

import (
	"os"
	"runtime"

	"github.com/humansintheloop-dev/isolarium/internal/git"
)

func PermissionFlags() []string {
	flags := []string{
		"--allow-cwd",
	}
	flags = append(flags, linuxSystemPathFlags()...)
	flags = append(flags, worktreeMainRepoDirFlags()...)
	return flags
}

func linuxSystemPathFlags() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	return []string{
		"--read", "/usr/lib/locale",
		"--override-deny", "/usr/lib/locale",
		"--read", "/usr/lib/jvm",
		"--override-deny", "/usr/lib/jvm",
		"--read", "/lib/x86_64-linux-gnu",
		"--read", "/usr/lib/x86_64-linux-gnu",
		"--override-deny", "/usr/lib/x86_64-linux-gnu",
		"--read", "/opt/hostedtoolcache",
	}
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
