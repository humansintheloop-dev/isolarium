package cli

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newInstallWorkflowToolsFromSourceCmd(rootCmd *cobra.Command, typeFlag *environmentType) *cobra.Command {
	return &cobra.Command{
		Use:   "install-workflow-tools-from-source <path>",
		Short: "Install workflow tools from a local directory, including uncommitted changes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if string(*typeFlag) == "nono" {
				return fmt.Errorf("install-workflow-tools-from-source is not supported with --type nono")
			}
			sourcePath := args[0]

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

			fmt.Println("Uninstalling existing i2code...")
			lima.UninstallI2Code(vmNameFlag)

			fmt.Println("Copying source to VM...")
			if err := lima.CopyDirToVM(vmNameFlag, sourcePath, "~/workflow-tools"); err != nil {
				return fmt.Errorf("failed to copy source to VM: %w", err)
			}

			fmt.Println("Installing custom plugins...")
			if err := lima.InstallPlugins(vmNameFlag); err != nil {
				return fmt.Errorf("failed to install custom plugins: %w", err)
			}

			fmt.Println("Installing i2code CLI...")
			if err := lima.InstallI2Code(vmNameFlag); err != nil {
				return fmt.Errorf("failed to install i2code CLI: %w", err)
			}

			fmt.Println("Workflow tools installed from source successfully")
			return nil
		},
	}
}
