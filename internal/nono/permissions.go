package nono

func PermissionFlags() []string {
	return []string{
		"--allow", ".",
		"--allow", "~/.claude/",
		"--allow-file", "~/.claude.json",
		"--allow", "~/.claude.json.lock",
		"--allow", "~/.claude.json.tmp.*",
		"--read-file", "~/Library/Keychains/login.keychain-db",
		"--allow", "~/.cache/uv",
		"--read", "~/.local/share/uv",
	}
}
