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
	archLib := archLibDir()
	return []string{
		"--read", "/usr/lib/locale",
		"--override-deny", "/usr/lib/locale",
		"--read", "/usr/lib/jvm",
		"--override-deny", "/usr/lib/jvm",
		"--read", "/lib/" + archLib,
		"--read", "/usr/lib/" + archLib,
		"--override-deny", "/usr/lib/" + archLib,
		"--read", "/opt/hostedtoolcache",
	}
}

func archLibDir() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64-linux-gnu"
	default:
		return "x86_64-linux-gnu"
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
