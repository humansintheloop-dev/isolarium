package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

// createEC2WithSpy drives the real create command against a spied backend, so
// what the command hands the backend can be asserted without an AWS account.
func createEC2WithSpy(t *testing.T, args ...string) *backendSpy {
	t.Helper()

	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs(append([]string{"create", "--type", "ec2"}, args...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return spy
}

func TestCreateCommand_EC2PassesExplicitNameToBackend(t *testing.T) {
	spy := createEC2WithSpy(t, "--name", "my-work")

	if !spy.createCalled {
		t.Fatal("expected backend.Create to be called")
	}
	if spy.createName != "my-work" {
		t.Errorf("expected name 'my-work', got '%s'", spy.createName)
	}
}

func TestCreateCommand_EC2UsesDefaultName(t *testing.T) {
	spy := createEC2WithSpy(t)

	if spy.createName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.createName)
	}
}

func TestCreateCommand_EC2GivesTheBackendARepositorySource(t *testing.T) {
	spy := createEC2WithSpy(t, "--name", "my-work")

	if spy.createOpts.Repository == nil {
		t.Fatal("create --type ec2 gave the backend no repository source, so it has nothing to clone")
	}
}

// The real EC2Backend now resolves the region before anything else, so an
// unset AWS_REGION is what proves create reached EC2Backend.Create rather than
// being rejected as an unknown type.
func createWithoutRegion(t *testing.T, args ...string) error {
	t.Helper()
	t.Setenv("AWS_REGION", "")
	rootCmd := NewRootCmd()
	rootCmd.SetArgs(append([]string{"create", "--env-file", filepath.Join(t.TempDir(), "absent.env")}, args...))
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected an error from the EC2 backend")
	}
	return err
}

func TestCreateCommand_EC2ResolvesToEC2Backend(t *testing.T) {
	err := createWithoutRegion(t, "--type", "ec2", "--name", "my-work")

	expectedMessage := `create "my-work": AWS_REGION is required for --type ec2; set it in .env.local`
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}

func TestCreateCommand_EC2DefaultNameReachesEC2Backend(t *testing.T) {
	err := createWithoutRegion(t, "--type", "ec2")

	expectedMessage := `create "isolarium-ec2": AWS_REGION is required for --type ec2; set it in .env.local`
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}

// The ec2 backend reads pid.yaml from the work directory, so a create that hands
// it none runs none of the project's own scripts.
func TestCreateCommand_EC2PassesTheCurrentDirectoryAsWorkDirectory(t *testing.T) {
	spy := createEC2WithSpy(t, "--name", "my-work")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving the current directory: %v", err)
	}
	if spy.createOpts.WorkDirectory != cwd {
		t.Errorf("create --type ec2 passed work directory %q, want %q", spy.createOpts.WorkDirectory, cwd)
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
