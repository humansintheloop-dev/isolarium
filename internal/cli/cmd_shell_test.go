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
	rootCmd.SetArgs([]string{"shell", "--type", "container"})
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
	rootCmd.SetArgs([]string{"shell", "--type", "container", "--name", "my-env"})
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

	rootCmd.SetArgs([]string{"shell", "--type", "container"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.openShellEnvVars["GH_TOKEN"] != "gho_test_token_123" {
		t.Errorf("expected GH_TOKEN 'gho_test_token_123', got '%s'", spy.openShellEnvVars["GH_TOKEN"])
	}
}
