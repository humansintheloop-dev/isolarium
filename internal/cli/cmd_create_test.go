package cli

import (
	"testing"

	"github.com/cer/isolarium/internal/backend"
	"github.com/spf13/cobra"
)

func disableAllRunHandlers(cmd *cobra.Command) {
	cmd.RunE = nil
	cmd.Run = nil
	for _, child := range cmd.Commands() {
		disableAllRunHandlers(child)
	}
}

func TestCreateCommand_TypeFlagDefaultsToVM(t *testing.T) {
	rootCmd := NewRootCmd()
	disableAllRunHandlers(rootCmd)
	rootCmd.SetArgs([]string{"create"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typeFlag := rootCmd.PersistentFlags().Lookup("type")
	if typeFlag == nil {
		t.Fatal("expected --type flag to exist on root command")
	}
	if typeFlag.Value.String() != "vm" {
		t.Errorf("expected --type default to be 'vm', got '%s'", typeFlag.Value.String())
	}
}

func TestCreateCommand_TypeFlagAcceptsContainer(t *testing.T) {
	rootCmd := NewRootCmd()
	disableAllRunHandlers(rootCmd)
	rootCmd.SetArgs([]string{"create", "--type", "container"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typeFlag := rootCmd.PersistentFlags().Lookup("type")
	if typeFlag.Value.String() != "container" {
		t.Errorf("expected --type to be 'container', got '%s'", typeFlag.Value.String())
	}
}

func TestCreateCommand_TypeFlagRejectsInvalidValue(t *testing.T) {
	rootCmd := NewRootCmd()
	disableAllRunHandlers(rootCmd)
	rootCmd.SetArgs([]string{"create", "--type", "invalid"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --type value")
	}
}

func TestCreateCommand_WorkDirectoryFlagDefaultsToCwd(t *testing.T) {
	rootCmd := NewRootCmd()
	disableAllRunHandlers(rootCmd)
	rootCmd.SetArgs([]string{"create", "--type", "container"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createCmd, _, err := rootCmd.Find([]string{"create"})
	if err != nil {
		t.Fatalf("failed to find create command: %v", err)
	}

	wdFlag := createCmd.Flags().Lookup("work-directory")
	if wdFlag == nil {
		t.Fatal("expected --work-directory flag to exist on create command")
	}
}

func TestCreateCommand_WorkDirectoryRejectedForVMType(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "vm", "--work-directory", "/some/path"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --work-directory used with --type vm")
	}
}

func TestCreateCommand_ContainerCallsBackendCreate(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "container"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spy.createCalled {
		t.Fatal("expected backend.Create to be called")
	}
	if spy.createName != "isolarium-container" {
		t.Errorf("expected name 'isolarium-container', got '%s'", spy.createName)
	}
}


func TestCreateCommand_ContainerPassesWorkDirectory(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "container", "--work-directory", "/my/project"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.createOpts.WorkDirectory != "/my/project" {
		t.Errorf("expected work directory '/my/project', got '%s'", spy.createOpts.WorkDirectory)
	}
}

func TestCreateCommand_ExplicitNameOverridesDefault(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"create", "--type", "container", "--name", "my-env"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.createName != "my-env" {
		t.Errorf("expected name 'my-env', got '%s'", spy.createName)
	}
}

type backendSpy struct {
	createCalled bool
	createName   string
	createOpts   backend.CreateOptions

	destroyCalled bool
	destroyName   string
}

func (b *backendSpy) Create(name string, opts backend.CreateOptions) error {
	b.createCalled = true
	b.createName = name
	b.createOpts = opts
	return nil
}

func (b *backendSpy) Destroy(name string) error {
	b.destroyCalled = true
	b.destroyName = name
	return nil
}

func (b *backendSpy) Exec(name string, envVars map[string]string, args []string) (int, error) {
	return 0, nil
}

func (b *backendSpy) ExecInteractive(name string, envVars map[string]string, args []string) (int, error) {
	return 0, nil
}

func (b *backendSpy) GetState(name string) string {
	return ""
}

func (b *backendSpy) CopyCredentials(name string, credentials string) error {
	return nil
}
