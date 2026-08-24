package cli

import (
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func ec2ShellWithSpy(t *testing.T, args ...string) *backendSpy {
	t.Helper()

	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		if envType != "ec2" {
			t.Fatalf("resolved backend for %q, want ec2", envType)
		}
		return spy, nil
	})
	rootCmd.SetArgs(append([]string{"shell", "--type", "ec2"}, args...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return spy
}

func TestShellCommand_EC2CallsBackendOpenShell(t *testing.T) {
	spy := ec2ShellWithSpy(t)

	if !spy.openShellCalled {
		t.Fatal("expected backend.OpenShell to be called")
	}
	if spy.openShellName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.openShellName)
	}
}

func TestShellCommand_EC2ExplicitNameOverridesDefault(t *testing.T) {
	spy := ec2ShellWithSpy(t, "--name", "my-work")

	if spy.openShellName != "my-work" {
		t.Errorf("expected name 'my-work', got '%s'", spy.openShellName)
	}
}

// TestShellCommand_EC2DoesNotCallCopyCredentials pins the ec2 shell to the
// backend's own conditional credential write, which the container copy would
// otherwise overwrite unconditionally.
func TestShellCommand_EC2DoesNotCallCopyCredentials(t *testing.T) {
	spy := ec2ShellWithSpy(t, "--copy-session")

	if spy.copyCredentialsCalled {
		t.Fatal("expected CopyCredentials NOT to be called for ec2 until credential copying lands")
	}
}
