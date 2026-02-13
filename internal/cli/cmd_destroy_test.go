package cli

import (
	"testing"

	"github.com/cer/isolarium/internal/backend"
)

func TestDestroyCommand_ContainerCallsBackendDestroy(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"destroy", "--type", "container"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.destroyCalled {
		t.Fatal("expected backend.Destroy to be called")
	}
	if spy.destroyName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.destroyName)
	}
}


func TestDestroyCommand_ExplicitNameOverridesDefault(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"destroy", "--type", "container", "--name", "my-env"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.destroyName != "my-env" {
		t.Errorf("expected name 'my-env', got '%s'", spy.destroyName)
	}
}
