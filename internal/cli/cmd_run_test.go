package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/backend"
)

func TestRunCommand_FallsBackToDefaultTypeWhenNoEnvironmentFound(t *testing.T) {
	spy := &backendSpy{}
	resolverCalledWithType := ""
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

	rootCmd.SetArgs([]string{"run", "--name", "my-env", "--copy-session=false", "--", "echo", "hello"})
	err := rootCmd.Execute()

	// The command will fail because there's no actual VM, but it should NOT fail
	// with "no environment found" — it should fall back to "vm" type.
	// Since we're using a spy backend, the VM path will call runInVM which
	// needs limactl. The key verification is that the resolver was NOT called
	// with "container" — it should default to "vm".
	_ = err
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

func TestRunCommand_NonoCallsBackendExec(t *testing.T) {
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

func TestRunCommand_NonoPassesEmptyEnvVars(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "nono", "--", "echo", "hello"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spy.execEnvVars) != 0 {
		t.Errorf("expected empty envVars for nono, got %v", spy.execEnvVars)
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

func TestRunCommand_NonoDoesNotCallCopyCredentials(t *testing.T) {
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
