package cli

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newDestroyCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver, envTypeResolver EnvironmentTypeResolver) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Destroy an isolated environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := resolveDefaultName(*nameFlag, string(*typeFlag), rootCmd)

			envType, err := resolveEnvType(rootCmd, typeFlag, name, envTypeResolver)
			if err != nil {
				return err
			}

			if envType == "vm" {
				return destroyVM(name)
			}

			b, err := resolver(envType)
			if err != nil {
				return err
			}

			return b.Destroy(name)
		},
	}
}

func destroyVM(name string) error {
	exists, err := lima.VMExists(name)
	if err != nil {
		return fmt.Errorf("failed to check VM status: %w", err)
	}
	if !exists {
		fmt.Println("no VM to destroy")
		return nil
	}

	fmt.Println("Destroying Lima VM...")
	if err := lima.DestroyVM(name); err != nil {
		return fmt.Errorf("failed to destroy VM: %w", err)
	}
	fmt.Println("VM destroyed successfully")
	return nil
}
