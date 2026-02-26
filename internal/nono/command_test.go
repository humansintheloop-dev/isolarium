package nono

import (
	"path/filepath"
	"testing"
)

func TestBuildRunCommandStartsWithNonoRun(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"})

	if cmd[0] != "nono" || cmd[1] != "run" {
		t.Errorf("expected command to start with [nono run], got %v", cmd[:2])
	}
}

func TestBuildRunCommandIncludesPermissionFlags(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"})

	home := homeDir()
	assertContainsSequence(t, cmd, "--allow", ".")
	assertContainsSequence(t, cmd, "--allow", filepath.Join(home, ".claude")+"/")
	assertContainsSequence(t, cmd, "--allow-file", filepath.Join(home, ".claude.json"))
	assertContainsSequence(t, cmd, "--read-file", filepath.Join(home, "Library", "Keychains", "login.keychain-db"))
	assertContainsSequence(t, cmd, "--read", filepath.Join(home, ".cache", "uv"))
	assertContainsSequence(t, cmd, "--read", filepath.Join(home, ".local", "share", "uv"))
}

func TestBuildRunCommandEndsWithSeparatorAndUserArgs(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"})

	separatorIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	if separatorIdx == -1 {
		t.Fatal("expected command to contain '--' separator")
	}

	userArgs := cmd[separatorIdx+1:]
	if len(userArgs) != 2 || userArgs[0] != "echo" || userArgs[1] != "hello" {
		t.Errorf("expected user args [echo hello], got %v", userArgs)
	}
}

func TestBuildRunCommandDoesNotIncludeExecFlag(t *testing.T) {
	cmd := BuildRunCommand([]string{"claude"})

	for _, v := range cmd {
		if v == "--exec" {
			t.Fatal("expected BuildRunCommand NOT to include --exec flag")
		}
	}
}

func TestBuildRunCommandInteractiveStartsWithNonoRun(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude"})

	if cmd[0] != "nono" || cmd[1] != "run" {
		t.Errorf("expected command to start with [nono run], got %v", cmd[:2])
	}
}

func TestBuildRunCommandInteractiveIncludesExecBeforeSeparator(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude"})

	execIdx := -1
	separatorIdx := -1
	for i, v := range cmd {
		if v == "--exec" && execIdx == -1 {
			execIdx = i
		}
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	if execIdx == -1 {
		t.Fatal("expected command to contain --exec flag")
	}
	if separatorIdx == -1 {
		t.Fatal("expected command to contain -- separator")
	}
	if execIdx >= separatorIdx {
		t.Errorf("expected --exec (index %d) to appear before -- (index %d)", execIdx, separatorIdx)
	}
}

func TestBuildRunCommandInteractiveIncludesPermissionFlags(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude"})

	assertContainsSequence(t, cmd, "--allow", ".")
}

func TestBuildRunCommandInteractiveEndsWithUserArgs(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude", "--verbose"})

	separatorIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	if separatorIdx == -1 {
		t.Fatal("expected command to contain '--' separator")
	}

	userArgs := cmd[separatorIdx+1:]
	if len(userArgs) != 2 || userArgs[0] != "claude" || userArgs[1] != "--verbose" {
		t.Errorf("expected user args [claude --verbose], got %v", userArgs)
	}
}

func TestBuildShellCommandStartsWithNonoShell(t *testing.T) {
	cmd := BuildShellCommand()

	if cmd[0] != "nono" || cmd[1] != "shell" {
		t.Errorf("expected command to start with [nono shell], got %v", cmd[:2])
	}
}

func TestBuildShellCommandIncludesPermissionFlags(t *testing.T) {
	cmd := BuildShellCommand()

	home := homeDir()
	assertContainsSequence(t, cmd, "--allow", ".")
	assertContainsSequence(t, cmd, "--allow", filepath.Join(home, ".claude")+"/")
	assertContainsSequence(t, cmd, "--allow-file", filepath.Join(home, ".claude.json"))
	assertContainsSequence(t, cmd, "--read-file", filepath.Join(home, "Library", "Keychains", "login.keychain-db"))
	assertContainsSequence(t, cmd, "--read", filepath.Join(home, ".cache", "uv"))
	assertContainsSequence(t, cmd, "--read", filepath.Join(home, ".local", "share", "uv"))
}

func TestBuildShellCommandDoesNotContainSeparatorOrExecFlag(t *testing.T) {
	cmd := BuildShellCommand()

	for _, v := range cmd {
		if v == "--" {
			t.Fatal("expected BuildShellCommand NOT to include '--' separator")
		}
		if v == "--exec" {
			t.Fatal("expected BuildShellCommand NOT to include '--exec' flag")
		}
	}
}

func TestBuildRunCommandPermissionFlagsBeforeSeparator(t *testing.T) {
	cmd := BuildRunCommand([]string{"ls"})

	separatorIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	knownFlags := map[string]bool{
		"--allow": true, "--allow-file": true, "--read-file": true, "--read": true,
	}
	knownValues := map[string]bool{
		".":            true,
		tempDir():      true,
		"/private/tmp": true,
		filepath.Join(homeDir(), ".claude") + "/":                                  true,
		filepath.Join(homeDir(), ".claude.json"):                                   true,
		filepath.Join(homeDir(), ".gitconfig"):                                     true,
		filepath.Join(homeDir(), ".config", "git"):                                 true,
		filepath.Join(homeDir(), ".config", "gh"):                                  true,
		filepath.Join(homeDir(), "Library", "Keychains", "login.keychain-db"):      true,
		filepath.Join(homeDir(), ".hitl"):                                          true,
		filepath.Join(homeDir(), ".cache", "uv"):                                   true,
		filepath.Join(homeDir(), ".local", "share", "uv"):                          true,
	}
	for _, flag := range worktreeMainRepoDirFlags() {
		knownValues[flag] = true
	}
	for i := 2; i < separatorIdx; i++ {
		flag := cmd[i]
		if !knownFlags[flag] && !knownValues[flag] {
			t.Errorf("unexpected flag before separator: %s", flag)
		}
	}
}
