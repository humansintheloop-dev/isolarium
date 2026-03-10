package cli

import (
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/spf13/cobra"
)

func stubBackendResolver(envType string) (backend.Backend, error) {
	return nil, nil
}

func executeRootCmdWithArgs(t *testing.T, args []string) (*string, *environmentType) {
	t.Helper()
	var capturedName string
	var capturedType environmentType

	root := newRootCmdWithResolvers(stubBackendResolver, nil)

	// Walk subcommands to find how name/type are wired — instead,
	// we'll read the flag values after parsing.
	root.SetArgs(args)
	// Silence output during tests
	root.SilenceUsage = true
	root.SilenceErrors = true

	// We need to trigger PersistentPreRunE, so add a dummy subcommand
	// that captures the flag values after PersistentPreRunE runs.
	captureCmd := newCaptureCmd(root, &capturedName, &capturedType)
	root.AddCommand(captureCmd)

	err := root.Execute()
	if err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	return &capturedName, &capturedType
}

func newCaptureCmd(rootCmd *cobra.Command, name *string, envType *environmentType) *cobra.Command {
	return &cobra.Command{
		Use: "capture-flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			nameVal, _ := rootCmd.PersistentFlags().GetString("name")
			*name = nameVal
			*envType = environmentType(rootCmd.PersistentFlags().Lookup("type").Value.String())
			return nil
		},
	}
}

func TestNameFlag_EnvVarOverridesDefault(t *testing.T) {
	t.Setenv("ISOLARIUM_NAME", "from-env")

	name, _ := executeRootCmdWithArgs(t, []string{"capture-flags"})

	if *name != "from-env" {
		t.Errorf("expected name 'from-env', got '%s'", *name)
	}
}

func TestNameFlag_ExplicitFlagOverridesEnvVar(t *testing.T) {
	t.Setenv("ISOLARIUM_NAME", "from-env")

	name, _ := executeRootCmdWithArgs(t, []string{"--name", "explicit", "capture-flags"})

	if *name != "explicit" {
		t.Errorf("expected name 'explicit', got '%s'", *name)
	}
}

func TestNameFlag_AbsentEnvVarFallsBackToDefault(t *testing.T) {
	// Ensure ISOLARIUM_NAME is not set
	t.Setenv("ISOLARIUM_NAME", "")

	name, _ := executeRootCmdWithArgs(t, []string{"capture-flags"})

	// Default is lima.GetVMName() which is "isolarium"
	if *name != "isolarium" {
		t.Errorf("expected name 'isolarium' (default), got '%s'", *name)
	}
}

func TestTypeFlag_EnvVarOverridesDefault(t *testing.T) {
	t.Setenv("ISOLARIUM_TYPE", "container")

	_, typ := executeRootCmdWithArgs(t, []string{"capture-flags"})

	if string(*typ) != "container" {
		t.Errorf("expected type 'container', got '%s'", string(*typ))
	}
}

func TestTypeFlag_ExplicitFlagOverridesEnvVar(t *testing.T) {
	t.Setenv("ISOLARIUM_TYPE", "container")

	_, typ := executeRootCmdWithArgs(t, []string{"--type", "nono", "capture-flags"})

	if string(*typ) != "nono" {
		t.Errorf("expected type 'nono', got '%s'", string(*typ))
	}
}

func TestTypeFlag_AbsentEnvVarFallsBackToDefault(t *testing.T) {
	t.Setenv("ISOLARIUM_TYPE", "")

	_, typ := executeRootCmdWithArgs(t, []string{"capture-flags"})

	// Default is "vm"
	if string(*typ) != "vm" {
		t.Errorf("expected type 'vm' (default), got '%s'", string(*typ))
	}
}

func TestTypeFlag_InvalidEnvVarReturnsError(t *testing.T) {
	t.Setenv("ISOLARIUM_TYPE", "invalid")

	root := newRootCmdWithResolvers(stubBackendResolver, nil)
	root.SetArgs([]string{"capture-flags"})
	root.SilenceUsage = true
	root.SilenceErrors = true

	var capturedName string
	var capturedType environmentType
	root.AddCommand(newCaptureCmd(root, &capturedName, &capturedType))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid ISOLARIUM_TYPE, got nil")
	}
}
