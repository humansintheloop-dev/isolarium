package cli

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newInstallToolsCmd(rootCmd *cobra.Command, typeFlag *environmentType) *cobra.Command {
	return &cobra.Command{
		Use:   "install-tools",
		Short: "Install workflow tools into the VM (retry after a failed create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if string(*typeFlag) == "nono" {
				return fmt.Errorf("install-tools is not supported with --type nono")
			}
			state := lima.GetVMState(vmNameFlag)
			if state == "none" {
				return fmt.Errorf("no VM exists; run 'isolarium create' first")
			}
			if state == "stopped" {
				fmt.Println("Starting stopped VM...")
				if err := lima.StartVM(vmNameFlag); err != nil {
					return err
				}
			}

			return (vmSetup{name: vmNameFlag}).installWorkflowTools()
		},
	}
}
