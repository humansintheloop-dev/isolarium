package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/spf13/cobra"
)

func TestRunCommand_FallsBackToDefaultTypeWhenNoEnvironmentFound(t *testing.T) {
	spy := &backendSpy{}
	resolverCalledWithType := ""
	vmPathReached := false
	rootCmd := newRootCmdWithResolvers(
		func(envType string) (backend.Backend, error) {
			resolverCalledWithType = envType
			return spy, nil
		},
		func(name string) (string, error) {
			return "", backend.ErrNoEnvironmentFound
		},
	)

	origExec := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origExec }()

	origRunInVM := runInVM
	runInVM = func(opts runOptions, cmd *cobra.Command) error {
		vmPathReached = true
		return nil
	}
	defer func() { runInVM = origRunInVM }()

	rootCmd.SetArgs([]string{"run", "--name", "my-env", "--copy-session=false", "--", "echo", "hello"})
	err := rootCmd.Execute()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vmPathReached {
		t.Error("expected VM path to be reached after fallback")
	}
	if resolverCalledWithType == "container" {
		t.Error("expected fallback to 'vm' type, but resolver was called with 'container'")
	}
}

func TestRunCommand_ContainerCallsBackendExec(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session=false", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
	if spy.execName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.execName)
	}
	if len(spy.execArgs) != 2 || spy.execArgs[0] != "echo" || spy.execArgs[1] != "hello" {
		t.Errorf("expected args [echo hello], got %v", spy.execArgs)
	}
}

func TestRunCommand_ContainerInteractiveCallsBackendExecInteractive(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session=false", "-i", "--", "bash"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.execInteractiveCalled {
		t.Fatal("expected backend.ExecInteractive to be called")
	}
	if spy.execInteractiveName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.execInteractiveName)
	}
}

func TestRunCommand_ContainerExplicitNameOverridesDefault(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session=false", "--name", "my-env", "--", "ls"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.execName != "my-env" {
		t.Errorf("expected name 'my-env', got '%s'", spy.execName)
	}
}

func TestRunCommand_ContainerInjectsGitHubTokenFromGhCli(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	origFn := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		if name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "token" {
			return []byte("gho_test_token_123\n"), nil
		}
		return nil, fmt.Errorf("unexpected command: %s", name)
	}
	defer func() { execCommandOutput = origFn }()

	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session=false", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.execEnvVars["GH_TOKEN"] != "gho_test_token_123" {
		t.Errorf("expected GH_TOKEN 'gho_test_token_123', got '%s'", spy.execEnvVars["GH_TOKEN"])
	}
}

func TestRunCommand_ContainerCopySessionCallsCopyCredentials(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	origExec := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origExec }()

	origKeychain := readKeychainCredentials
	readKeychainCredentials = func() (string, error) {
		return `{"token":"test-creds"}`, nil
	}
	defer func() { readKeychainCredentials = origKeychain }()

	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.copyCredentialsCalled {
		t.Fatal("expected backend.CopyCredentials to be called")
	}
	if spy.copyCredentialsName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.copyCredentialsName)
	}
	if spy.copyCredentialsCredentials != `{"token":"test-creds"}` {
		t.Errorf("expected credentials %q, got %q", `{"token":"test-creds"}`, spy.copyCredentialsCredentials)
	}
}

func TestRunCommand_ContainerNoCopySessionSkipsCopyCredentials(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	origExec := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origExec }()

	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session=false", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.copyCredentialsCalled {
		t.Fatal("expected backend.CopyCredentials NOT to be called when --copy-session=false")
	}
}

func TestRunCommand_ContainerOmitsTokenWhenGhCliNotAvailable(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	origFn := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origFn }()

	rootCmd.SetArgs([]string{"run", "--type", "container", "--copy-session=false", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, exists := spy.execEnvVars["GH_TOKEN"]; exists {
		t.Error("expected GH_TOKEN to not be set when gh cli is unavailable")
	}
}

func TestRunCommand_AutoDetectsContainerWhenTypeNotProvided(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolvers(
		func(envType string) (backend.Backend, error) {
			return spy, nil
		},
		func(name string) (string, error) {
			return "container", nil
		},
	)

	origExec := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origExec }()

	rootCmd.SetArgs([]string{"run", "--name", "my-env", "--copy-session=false", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
	if spy.execName != "my-env" {
		t.Errorf("expected name 'my-env', got '%s'", spy.execName)
	}
}

func stubMintGitHubToken(t *testing.T) {
	t.Helper()
	orig := mintGitHubToken
	mintGitHubToken = func() (string, error) { return "test-token", nil }
	t.Cleanup(func() { mintGitHubToken = orig })
}

func TestRunCommand_NonoCallsBackendExec(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
	if spy.execName != "isolarium-nono" {
		t.Errorf("expected name 'isolarium-nono', got '%s'", spy.execName)
	}
	if len(spy.execArgs) != 2 || spy.execArgs[0] != "echo" || spy.execArgs[1] != "hello" {
		t.Errorf("expected args [echo hello], got %v", spy.execArgs)
	}
}

func TestRunCommand_NonoFailsWhenGitHubAppNotConfigured(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "")

	rootCmd.SetArgs([]string{"run", "--type", "nono", "--", "echo", "hello"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error when GitHub App is not configured")
	}
	if !strings.Contains(err.Error(), "GitHub App not configured") {
		t.Errorf("expected error about GitHub App not configured, got: %v", err)
	}
}

func TestRunCommand_NonoRejectsCopySessionWhenExplicitlySet(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--copy-session", "--", "echo", "hello"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --copy-session is explicitly set with --type nono")
	}
	if !strings.Contains(err.Error(), "--copy-session is not supported with --type nono") {
		t.Errorf("expected error message about --copy-session not supported, got: %v", err)
	}
}

func TestRunCommand_NonoRejectsFreshLoginWhenExplicitlySet(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--fresh-login", "--", "echo", "hello"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --fresh-login is explicitly set with --type nono")
	}
	if !strings.Contains(err.Error(), "--fresh-login is not supported with --type nono") {
		t.Errorf("expected error message about --fresh-login not supported, got: %v", err)
	}
}

func TestRunCommand_NonoInteractiveCallsBackendExecInteractive(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "-i", "--", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.execInteractiveCalled {
		t.Fatal("expected backend.ExecInteractive to be called")
	}
	if spy.execInteractiveName != "isolarium-nono" {
		t.Errorf("expected name 'isolarium-nono', got '%s'", spy.execInteractiveName)
	}
	if len(spy.execInteractiveArgs) != 1 || spy.execInteractiveArgs[0] != "claude" {
		t.Errorf("expected args [claude], got %v", spy.execInteractiveArgs)
	}
}

func TestRunCommand_NonoNonInteractiveCallsBackendExec(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
	if spy.execInteractiveCalled {
		t.Fatal("expected backend.ExecInteractive NOT to be called for non-interactive nono run")
	}
}

func TestRunCommand_NonoReadFlagSetsExtraReadPaths(t *testing.T) {
	stubMintGitHubToken(t)
	var calledExtraReadPaths []string
	nb := &backend.NonoBackend{
		ExecFunc: func(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error) {
			calledExtraReadPaths = extraReadPaths
			return 0, nil
		},
	}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return nb, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--read", "/path/one", "--read", "/path/two", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(calledExtraReadPaths) != 2 || calledExtraReadPaths[0] != "/path/one" || calledExtraReadPaths[1] != "/path/two" {
		t.Errorf("expected extraReadPaths [/path/one /path/two], got %v", calledExtraReadPaths)
	}
}

func containerRunWithSpyState(t *testing.T, state string, envFlags []string, runArgs []string) *backendSpy {
	t.Helper()
	spy := &backendSpy{state: state}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	origFn := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origFn }()

	var args []string
	for _, ef := range envFlags {
		args = append(args, "--env", ef)
	}
	args = append(args, "run", "--type", "container", "--copy-session=false")
	args = append(args, runArgs...)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return spy
}

func containerRunWithEnvFlags(t *testing.T, envFlags []string, runArgs []string) *backendSpy {
	t.Helper()
	return containerRunWithSpyState(t, "", envFlags, runArgs)
}

func TestRunCommand_ContainerPassesEnvFlagVarsToBackendExec(t *testing.T) {
	spy := containerRunWithEnvFlags(t, []string{"FOO=bar", "BAZ=qux"}, []string{"--", "env"})

	if spy.execEnvVars["FOO"] != "bar" {
		t.Errorf("expected FOO='bar', got '%s'", spy.execEnvVars["FOO"])
	}
	if spy.execEnvVars["BAZ"] != "qux" {
		t.Errorf("expected BAZ='qux', got '%s'", spy.execEnvVars["BAZ"])
	}
}

func TestRunCommand_ContainerPassesEnvFlagVarsToBackendExecInteractive(t *testing.T) {
	spy := containerRunWithEnvFlags(t, []string{"MY_VAR=hello"}, []string{"-i", "--", "bash"})

	if spy.execInteractiveEnvVars["MY_VAR"] != "hello" {
		t.Errorf("expected MY_VAR='hello', got '%s'", spy.execInteractiveEnvVars["MY_VAR"])
	}
}

func TestRunCommand_VMPassesEnvFlagVarsToBackend(t *testing.T) {
	stubMintGitHubToken(t)

	origParsed := parsedEnvVars
	parsedEnvVars = map[string]string{"FOO": "bar", "BAZ": "qux"}
	defer func() { parsedEnvVars = origParsed }()

	envVars, err := buildVMEnvVars(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envVars["FOO"] != "bar" {
		t.Errorf("expected FOO='bar', got '%s'", envVars["FOO"])
	}
	if envVars["BAZ"] != "qux" {
		t.Errorf("expected BAZ='qux', got '%s'", envVars["BAZ"])
	}
}

func TestRunCommand_NonoPassesEnvFlagVarsToBackend(t *testing.T) {
	stubMintGitHubToken(t)

	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"--env", "FOO=bar", "--env", "BAZ=qux", "run", "--type", "nono", "--", "printenv", "FOO"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.execEnvVars["FOO"] != "bar" {
		t.Errorf("expected FOO='bar', got '%s'", spy.execEnvVars["FOO"])
	}
	if spy.execEnvVars["BAZ"] != "qux" {
		t.Errorf("expected BAZ='qux', got '%s'", spy.execEnvVars["BAZ"])
	}
}

func stubLoadRunEnvVars(t *testing.T, vars map[string]string) {
	t.Helper()
	orig := loadRunEnvVars
	loadRunEnvVars = func(isolationType string) (map[string]string, error) {
		return vars, nil
	}
	t.Cleanup(func() { loadRunEnvVars = orig })
}

func TestRunCommand_ContainerInjectsRunEnvVarsFromPidYaml(t *testing.T) {
	stubLoadRunEnvVars(t, map[string]string{"PID_VAR": "pid_value", "OTHER_VAR": "other_value"})
	spy := containerRunWithEnvFlags(t, nil, []string{"--", "env"})

	if spy.execEnvVars["PID_VAR"] != "pid_value" {
		t.Errorf("expected PID_VAR='pid_value', got '%s'", spy.execEnvVars["PID_VAR"])
	}
	if spy.execEnvVars["OTHER_VAR"] != "other_value" {
		t.Errorf("expected OTHER_VAR='other_value', got '%s'", spy.execEnvVars["OTHER_VAR"])
	}
}

func TestRunCommand_NonoInjectsRunEnvVarsFromPidYaml(t *testing.T) {
	stubMintGitHubToken(t)
	stubLoadRunEnvVars(t, map[string]string{"PID_VAR": "pid_value"})

	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--", "printenv", "PID_VAR"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.execEnvVars["PID_VAR"] != "pid_value" {
		t.Errorf("expected PID_VAR='pid_value', got '%s'", spy.execEnvVars["PID_VAR"])
	}
}

func TestRunCommand_VMInjectsRunEnvVarsFromPidYaml(t *testing.T) {
	stubMintGitHubToken(t)
	stubLoadRunEnvVars(t, map[string]string{"PID_VAR": "pid_value"})

	origParsed := parsedEnvVars
	parsedEnvVars = map[string]string{}
	defer func() { parsedEnvVars = origParsed }()

	envVars, err := buildVMEnvVars(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envVars["PID_VAR"] != "pid_value" {
		t.Errorf("expected PID_VAR='pid_value', got '%s'", envVars["PID_VAR"])
	}
}

func TestRunCommand_EnvFlagOverridesRunEnvFromPidYaml(t *testing.T) {
	stubLoadRunEnvVars(t, map[string]string{"FOO": "from_pid"})
	spy := containerRunWithEnvFlags(t, []string{"FOO=from_flag"}, []string{"--", "env"})

	if spy.execEnvVars["FOO"] != "from_flag" {
		t.Errorf("expected --env flag to override run.env: got '%s'", spy.execEnvVars["FOO"])
	}
}

func containerRunWithCreateFlag(t *testing.T, state string, extraArgs ...string) *backendSpy {
	t.Helper()
	runArgs := append([]string{"--create"}, extraArgs...)
	runArgs = append(runArgs, "--", "echo", "hello")
	spy := containerRunWithSpyState(t, state, nil, runArgs)
	return spy
}

func TestRunCommand_CreateFlagCreatesContainerWhenStateIsNone(t *testing.T) {
	spy := containerRunWithCreateFlag(t, "none")

	if !spy.createCalled {
		t.Fatal("expected backend.Create to be called when --create is set and state is none")
	}
	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called after create")
	}
}

func TestRunCommand_CreateFlagSkipsCreateWhenEnvironmentExists(t *testing.T) {
	spy := containerRunWithCreateFlag(t, "running")

	if spy.createCalled {
		t.Fatal("expected backend.Create NOT to be called when environment already exists")
	}
}

func runWithSpy(t *testing.T, spy *backendSpy, args []string) {
	t.Helper()
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCommand_CreateFlagCreatesNonoWhenStateIsNone(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{state: "none"}
	runWithSpy(t, spy, []string{"run", "--type", "nono", "--create", "--", "echo", "hello"})

	if !spy.createCalled {
		t.Fatal("expected backend.Create to be called when --create is set and state is none")
	}
}

func TestRunCommand_WorkDirectoryRequiresCreateFlag(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "container", "--work-directory", "/tmp/foo", "--copy-session=false", "--", "echo", "hello"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --work-directory is set without --create")
	}
	if !strings.Contains(err.Error(), "--work-directory requires --create") {
		t.Errorf("expected error about --work-directory requiring --create, got: %v", err)
	}
}

func TestRunCommand_CreateFlagPassesWorkDirectoryToBackend(t *testing.T) {
	spy := containerRunWithCreateFlag(t, "none", "--work-directory", "/tmp/foo")

	if spy.createOpts.WorkDirectory != "/tmp/foo" {
		t.Errorf("expected work directory '/tmp/foo', got '%s'", spy.createOpts.WorkDirectory)
	}
}

func TestRunCommand_NonoDoesNotCallCopyCredentials(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.copyCredentialsCalled {
		t.Fatal("expected CopyCredentials NOT to be called for nono")
	}
}
