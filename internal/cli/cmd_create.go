package cli

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/backend"
	"github.com/spf13/cobra"
)

const defaultContainerName = "isolarium-container"

func newCreateCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver) *cobra.Command {
	var workDirFlag string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an isolated environment for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			envType := string(*typeFlag)

			if workDirectoryExplicitlySet(cmd) && envType == "vm" {
				return fmt.Errorf("--work-directory is only supported with --type container")
			}

			name := resolveDefaultName(*nameFlag, envType, rootCmd)

			if envType == "vm" {
				return createAndSetupVM(name)
			}

			b, err := resolver(envType)
			if err != nil {
				return err
			}

			opts := backend.CreateOptions{
				WorkDirectory: workDirFlag,
			}
			return b.Create(name, opts)
		},
	}

	cwd, _ := os.Getwd()
	cmd.Flags().StringVar(&workDirFlag, "work-directory", cwd, "Work directory to mount (container mode only)")

	return cmd
}

func workDirectoryExplicitlySet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("work-directory")
}

func resolveDefaultName(nameFlag string, envType string, rootCmd *cobra.Command) string {
	if rootCmd.PersistentFlags().Changed("name") {
		return nameFlag
	}
	if envType == "container" {
		return defaultContainerName
	}
	return nameFlag
}
