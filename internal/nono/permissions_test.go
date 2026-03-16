package nono

import (
	"path/filepath"
	"testing"
)

func TestPermissionFlagsContainsAllowCwd(t *testing.T) {
	flags := PermissionFlags()

	assertContainsFlag(t, flags, "--allow-cwd")
}


func TestMarketplaceReadFlagsIncludesPathsOutsideClaudeDir(t *testing.T) {
	claudeDir := "/home/user/.claude"
	marketplaces := map[string]MarketplaceEntry{
		"internal": {InstallLocation: filepath.Join(claudeDir, "plugins", "marketplaces", "internal")},
		"external": {InstallLocation: "/opt/shared/my-marketplace"},
	}

	flags := marketplaceReadFlags(claudeDir, marketplaces)

	assertContainsSequence(t, flags, "--read", "/opt/shared/my-marketplace")
}

func TestMarketplaceReadFlagsExcludesPathsInsideClaudeDir(t *testing.T) {
	claudeDir := "/home/user/.claude"
	marketplaces := map[string]MarketplaceEntry{
		"internal": {InstallLocation: filepath.Join(claudeDir, "plugins", "marketplaces", "internal")},
	}

	flags := marketplaceReadFlags(claudeDir, marketplaces)

	if len(flags) != 0 {
		t.Errorf("expected no flags for paths inside claude dir, got %v", flags)
	}
}

func TestMarketplaceReadFlagsHandlesMultipleExternalPaths(t *testing.T) {
	claudeDir := "/home/user/.claude"
	marketplaces := map[string]MarketplaceEntry{
		"internal": {InstallLocation: filepath.Join(claudeDir, "plugins", "marketplaces", "foo")},
		"ext1":     {InstallLocation: "/opt/marketplace-a"},
		"ext2":     {InstallLocation: "/opt/marketplace-b"},
	}

	flags := marketplaceReadFlags(claudeDir, marketplaces)

	assertContainsSequence(t, flags, "--read", "/opt/marketplace-a")
	assertContainsSequence(t, flags, "--read", "/opt/marketplace-b")
}

func assertContainsFlag(t *testing.T, slice []string, flag string) {
	t.Helper()
	for _, v := range slice {
		if v == flag {
			return
		}
	}
	t.Errorf("expected flags to contain %s, got %v", flag, slice)
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
