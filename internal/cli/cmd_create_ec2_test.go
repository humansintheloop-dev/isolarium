package cli

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func TestCreateCommand_EC2PassesExplicitNameToBackend(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "ec2", "--name", "my-work"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.createCalled {
		t.Fatal("expected backend.Create to be called")
	}
	if spy.createName != "my-work" {
		t.Errorf("expected name 'my-work', got '%s'", spy.createName)
	}
}

func TestCreateCommand_EC2UsesDefaultName(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "ec2"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.createName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.createName)
	}
}

func TestCreateCommand_EC2ResolvesToEC2Backend(t *testing.T) {
	rootCmd := NewRootCmd()
	rootCmd.SetArgs([]string{"create", "--type", "ec2", "--name", "my-work"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected not-yet-implemented error from the EC2 backend")
	}

	expectedMessage := `create "my-work": not yet implemented for --type ec2`
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}

func TestCreateCommand_EC2DefaultNameReachesEC2Backend(t *testing.T) {
	rootCmd := NewRootCmd()
	rootCmd.SetArgs([]string{"create", "--type", "ec2"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected not-yet-implemented error from the EC2 backend")
	}

	expectedMessage := `create "isolarium-ec2": not yet implemented for --type ec2`
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}

func TestCreateCommand_WorkDirectoryRejectedForEC2Type(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "ec2", "--work-directory", "/some/path"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --work-directory used with --type ec2")
	}

	expectedMessage := "--work-directory is not supported with --type ec2"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}
