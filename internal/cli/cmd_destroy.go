package cli

import (
	"fmt"

	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Delete the Lima VM and all its contents",
		RunE: func(cmd *cobra.Command, args []string) error {
			exists, err := lima.VMExists(vmNameFlag)
			if err != nil {
				return fmt.Errorf("failed to check VM status: %w", err)
			}
			if !exists {
				fmt.Println("no VM to destroy")
				return nil
			}

			fmt.Println("Destroying Lima VM...")
			if err := lima.DestroyVM(vmNameFlag); err != nil {
				return fmt.Errorf("failed to destroy VM: %w", err)
			}
			fmt.Println("VM destroyed successfully")
			return nil
		},
	}
}
