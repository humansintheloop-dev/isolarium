package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func ec2RunWithSpy(t *testing.T, args ...string) *backendSpy {
	t.Helper()
	stubMintGitHubToken(t)
	stubKeychainCredentials(t, ec2HostCredentials, nil)
	spy := &backendSpy{}
	runWithSpy(t, spy, append([]string{"run", "--type", "ec2"}, args...))
	return spy
}

const ec2HostCredentials = `{"claudeAiOauth":{"expiresAt":2000}}`

func TestRunCommand_EC2CallsBackendExec(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "echo", "hello")

	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
	if spy.execName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.execName)
	}
	assertArgsEqual(t, "exec args", spy.execArgs, []string{"echo", "hello"})
}

func TestRunCommand_EC2InteractiveCallsBackendExecInteractive(t *testing.T) {
	spy := ec2RunWithSpy(t, "-i", "--", "claude")

	if !spy.execInteractiveCalled {
		t.Fatal("expected backend.ExecInteractive to be called")
	}
	if spy.execCalled {
		t.Fatal("expected backend.Exec NOT to be called for an interactive ec2 run")
	}
	if spy.execInteractiveName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.execInteractiveName)
	}
}

func TestRunCommand_EC2InjectsRunEnvVarsFromPidYaml(t *testing.T) {
	stubLoadRunEnvVars(t, map[string]string{"PID_VAR": "pid_value"})
	spy := ec2RunWithSpy(t, "--", "printenv", "PID_VAR")

	if spy.execEnvVars["PID_VAR"] != "pid_value" {
		t.Errorf("expected PID_VAR='pid_value', got '%s'", spy.execEnvVars["PID_VAR"])
	}
}

func TestRunCommand_EC2InjectsMintedGitHubToken(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "gh", "auth", "status")

	if spy.execEnvVars["GH_TOKEN"] != "test-token" {
		t.Errorf("expected GH_TOKEN 'test-token', got '%s'", spy.execEnvVars["GH_TOKEN"])
	}
	if spy.execEnvVars["GIT_TOKEN"] != "test-token" {
		t.Errorf("expected GIT_TOKEN 'test-token', got '%s'", spy.execEnvVars["GIT_TOKEN"])
	}
}

// TestRunCommand_EC2RewritesTheHTTPSOriginWithTheMintedToken covers the other
// half of keeping the clone token off the instance: create leaves origin
// credential-free, so each run has to supply a token of its own for git to push.
func TestRunCommand_EC2RewritesTheHTTPSOriginWithTheMintedToken(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "git", "push")

	want := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "url.https://x-access-token:test-token@github.com/.insteadOf",
		"GIT_CONFIG_VALUE_0": "https://github.com/",
	}
	for key, value := range want {
		if spy.execEnvVars[key] != value {
			t.Errorf("expected %s=%q, got %q", key, value, spy.execEnvVars[key])
		}
	}
}

func ec2RunWithState(t *testing.T, state string, args ...string) *backendSpy {
	t.Helper()
	stubMintGitHubToken(t)
	stubKeychainCredentials(t, ec2HostCredentials, nil)
	spy := &backendSpy{state: state}
	runWithSpy(t, spy, append([]string{"run", "--type", "ec2"}, args...))
	return spy
}

// i2code launches every environment with `run --create`, so ec2 has to create on
// demand like the other types rather than insisting on a separate create.
func TestRunCommand_EC2CreateFlagCreatesWhenStateIsNone(t *testing.T) {
	spy := ec2RunWithState(t, "none", "--create", "--", "echo", "hello")

	if !spy.createCalled {
		t.Fatal("expected backend.Create to be called when the environment does not exist")
	}
	if spy.createName != "isolarium-ec2" {
		t.Errorf("expected create name 'isolarium-ec2', got '%s'", spy.createName)
	}
	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called after creating")
	}
}

// The create has to be the same one `isolarium create --type ec2` performs: the
// backend reads pid.yaml from the work directory and clones from the repository
// source, so a run that left either out would produce a half-built environment.
func TestRunCommand_EC2CreateFlagPassesCurrentDirectoryAndRepositorySource(t *testing.T) {
	spy := ec2RunWithState(t, "none", "--create", "--", "echo", "hello")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving the current directory: %v", err)
	}
	if spy.createOpts.WorkDirectory != cwd {
		t.Errorf("run --create --type ec2 passed work directory %q, want %q", spy.createOpts.WorkDirectory, cwd)
	}
	if spy.createOpts.Repository == nil {
		t.Fatal("run --create --type ec2 gave the backend no repository source, so it has nothing to clone")
	}
}

func TestRunCommand_EC2CreateFlagSkipsCreateWhenEnvironmentExists(t *testing.T) {
	spy := ec2RunWithState(t, "running", "--create", "--", "echo", "hello")

	if spy.createCalled {
		t.Fatal("expected backend.Create NOT to be called when the environment already exists")
	}
	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
}

func TestRunCommand_EC2RejectsWorkDirectory(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{state: "none"}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "ec2", "--create", "--work-directory", "/some/path", "--", "echo", "hello"})

	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error when --work-directory is used with --type ec2")
	}
	expected := "--work-directory is not supported with --type ec2"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error %q, got: %v", expected, err)
	}
	if spy.createCalled {
		t.Error("expected backend.Create NOT to be called")
	}
	if spy.execCalled {
		t.Error("expected backend.Exec NOT to be called")
	}
}

// TestRunCommand_EC2HandsTheHostCredentialsToTheBackend covers the run side of
// spec 3.11: the CLI always offers the host blob, and the backend is what
// decides whether the instance's own copy is fresher.
func TestRunCommand_EC2HandsTheHostCredentialsToTheBackend(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "echo", "hello")

	if !spy.copyCredentialsCalled {
		t.Fatal("expected CopyCredentials to be called for ec2")
	}
	if spy.copyCredentialsName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.copyCredentialsName)
	}
	if spy.copyCredentialsCredentials != ec2HostCredentials {
		t.Errorf("expected credentials %q, got %q", ec2HostCredentials, spy.copyCredentialsCredentials)
	}
}

func TestRunCommand_EC2SkipsCopyCredentialsWhenCopySessionDisabled(t *testing.T) {
	spy := ec2RunWithSpy(t, "--copy-session=false", "--", "echo", "hello")

	if spy.copyCredentialsCalled {
		t.Fatal("expected CopyCredentials NOT to be called when --copy-session is off")
	}
	if !spy.execCalled {
		t.Fatal("expected the command to run even without a credential copy")
	}
}

func TestRunCommand_EC2SurfacesCopyCredentialsFailure(t *testing.T) {
	stubMintGitHubToken(t)
	stubKeychainCredentials(t, ec2HostCredentials, nil)
	spy := &backendSpy{copyCredentialsErr: fmt.Errorf("instance unreachable")}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "ec2", "--", "echo", "hello"})

	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected run to fail when copying credentials fails")
	}
	if !strings.Contains(err.Error(), "failed to copy credentials: instance unreachable") {
		t.Errorf("expected wrapped backend error, got: %v", err)
	}
	if spy.execCalled {
		t.Fatal("expected the command NOT to run after a credential copy failure")
	}
}
