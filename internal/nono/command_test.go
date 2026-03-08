package nono

import (
	"testing"
)

func TestBuildRunCommandStartsWithNonoWrapAndProfile(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"}, nil)

	assertCommandPrefix(t, cmd, "nono", "wrap", "--profile", getProfilePath())
}

func TestBuildRunCommandIncludesPermissionFlags(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"}, nil)

	assertContainsFlag(t, cmd, "--allow-cwd")
}

func TestBuildRunCommandEndsWithSeparatorAndUserArgs(t *testing.T) {
	cmd := BuildRunCommand([]string{"echo", "hello"}, nil)

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
	cmd := BuildRunCommand([]string{"claude"}, nil)

	for _, v := range cmd {
		if v == "--exec" {
			t.Fatal("expected BuildRunCommand NOT to include --exec flag")
		}
		if v == "run" {
			t.Fatal("expected BuildRunCommand to use wrap, not run")
		}
	}
}

func TestBuildRunCommandInteractiveStartsWithNonoRunAndProfile(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude"}, nil)

	assertCommandPrefix(t, cmd, "nono", "run", "--profile", getProfilePath())
}

func TestBuildRunCommandInteractiveIncludesExecBeforeSeparator(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude"}, nil)

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
	cmd := BuildRunCommandInteractive([]string{"claude"}, nil)

	assertContainsFlag(t, cmd, "--allow-cwd")
}

func TestBuildRunCommandInteractiveEndsWithUserArgs(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude", "--verbose"}, nil)

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

func TestBuildShellCommandStartsWithNonoShellAndProfile(t *testing.T) {
	cmd := BuildShellCommand()

	assertCommandPrefix(t, cmd, "nono", "shell", "--profile", getProfilePath())
}

func TestBuildShellCommandIncludesPermissionFlags(t *testing.T) {
	cmd := BuildShellCommand()

	assertContainsFlag(t, cmd, "--allow-cwd")
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
	cmd := BuildRunCommand([]string{"ls"}, nil)

	separatorIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
	}

	knownFlags := map[string]bool{
		"--allow": true, "--read": true, "--profile": true, "--allow-cwd": true, "--override-deny": true,
	}
	knownValues := map[string]bool{
		getProfilePath(): true,
	}
	for _, flag := range linuxSystemPathFlags() {
		knownValues[flag] = true
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

func TestBuildRunCommandIncludesExtraReadPaths(t *testing.T) {
	cmd := BuildRunCommand([]string{"ls"}, []string{"/extra/path1", "/extra/path2"})

	assertContainsSequence(t, cmd, "--read", "/extra/path1")
	assertContainsSequence(t, cmd, "--read", "/extra/path2")
}

func TestBuildRunCommandInteractiveIncludesExtraReadPaths(t *testing.T) {
	cmd := BuildRunCommandInteractive([]string{"claude"}, []string{"/extra/path"})

	assertContainsSequence(t, cmd, "--read", "/extra/path")
}

func TestBuildRunCommandExtraReadPathsBeforeSeparator(t *testing.T) {
	cmd := BuildRunCommand([]string{"ls"}, []string{"/extra/path"})

	separatorIdx := -1
	extraReadIdx := -1
	for i, v := range cmd {
		if v == "--" {
			separatorIdx = i
			break
		}
		if v == "/extra/path" {
			extraReadIdx = i
		}
	}

	if extraReadIdx == -1 {
		t.Fatal("expected extra read path to appear in command")
	}
	if extraReadIdx >= separatorIdx {
		t.Errorf("expected extra read path (index %d) before separator (index %d)", extraReadIdx, separatorIdx)
	}
}

func assertCommandPrefix(t *testing.T, cmd []string, expected ...string) {
	t.Helper()
	if len(cmd) < len(expected) {
		t.Fatalf("command too short: got %v, expected prefix %v", cmd, expected)
	}
	for i, want := range expected {
		if cmd[i] != want {
			t.Errorf("expected command prefix %v, got %v", expected, cmd[:len(expected)])
			return
		}
	}
}
