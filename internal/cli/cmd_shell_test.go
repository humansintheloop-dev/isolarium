package cli

import (
	"fmt"
	"testing"

	"github.com/cer/isolarium/internal/backend"
)

func TestShellCommand_ContainerCallsBackendOpenShell(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"shell", "--type", "container", "--copy-session=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.openShellCalled {
		t.Fatal("expected backend.OpenShell to be called")
	}
	if spy.openShellName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.openShellName)
	}
}

func TestShellCommand_ContainerExplicitNameOverridesDefault(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"shell", "--type", "container", "--copy-session=false", "--name", "my-env"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.openShellName != "my-env" {
		t.Errorf("expected name 'my-env', got '%s'", spy.openShellName)
	}
}

func TestShellCommand_ContainerInjectsGitHubToken(t *testing.T) {
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

	rootCmd.SetArgs([]string{"shell", "--type", "container", "--copy-session=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.openShellEnvVars["GH_TOKEN"] != "gho_test_token_123" {
		t.Errorf("expected GH_TOKEN 'gho_test_token_123', got '%s'", spy.openShellEnvVars["GH_TOKEN"])
	}
}

func TestShellCommand_ContainerCopySessionCallsCopyCredentials(t *testing.T) {
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
		return `{"token":"shell-creds"}`, nil
	}
	defer func() { readKeychainCredentials = origKeychain }()

	rootCmd.SetArgs([]string{"shell", "--type", "container", "--copy-session"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.copyCredentialsCalled {
		t.Fatal("expected backend.CopyCredentials to be called")
	}
	if spy.copyCredentialsName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.copyCredentialsName)
	}
	if spy.copyCredentialsCredentials != `{"token":"shell-creds"}` {
		t.Errorf("expected credentials %q, got %q", `{"token":"shell-creds"}`, spy.copyCredentialsCredentials)
	}
}

func TestShellCommand_ContainerNoCopySessionSkipsCopyCredentials(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})

	origExec := execCommandOutput
	execCommandOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("gh not found")
	}
	defer func() { execCommandOutput = origExec }()

	rootCmd.SetArgs([]string{"shell", "--type", "container", "--copy-session=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.copyCredentialsCalled {
		t.Fatal("expected backend.CopyCredentials NOT to be called when --copy-session=false")
	}
}

func TestShellCommand_AutoDetectsContainerWhenTypeNotProvided(t *testing.T) {
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

	rootCmd.SetArgs([]string{"shell", "--name", "my-env", "--copy-session=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.openShellCalled {
		t.Fatal("expected backend.OpenShell to be called")
	}
	if spy.openShellName != "my-env" {
		t.Errorf("expected name 'my-env', got '%s'", spy.openShellName)
	}
}
