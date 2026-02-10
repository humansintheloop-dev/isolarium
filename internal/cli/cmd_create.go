package cli

import (
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create and start a Lima VM for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createAndSetupVM(vmNameFlag)
		},
	}
}
