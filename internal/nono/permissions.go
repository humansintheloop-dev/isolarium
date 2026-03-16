package nono

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/git"
)

type MarketplaceEntry struct {
	InstallLocation string `json:"installLocation"`
}

func PermissionFlags() []string {
	flags := []string{
		"--allow-cwd",
	}
	flags = append(flags, linuxSystemPathFlags()...)
	flags = append(flags, worktreeMainRepoDirFlags()...)
	flags = append(flags, claudePluginMarketplaceFlags()...)
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

func claudePluginMarketplaceFlags() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	claudeDir := filepath.Join(homeDir, ".claude")
	data, err := os.ReadFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"))
	if err != nil {
		return nil
	}
	var marketplaces map[string]MarketplaceEntry
	if err := json.Unmarshal(data, &marketplaces); err != nil {
		return nil
	}
	return marketplaceReadFlags(claudeDir, marketplaces)
}

func marketplaceReadFlags(claudeDir string, marketplaces map[string]MarketplaceEntry) []string {
	var flags []string
	for _, entry := range marketplaces {
		if !strings.HasPrefix(entry.InstallLocation, claudeDir+"/") {
			flags = append(flags, "--read", entry.InstallLocation)
		}
	}
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
