package cli

import (
	"fmt"
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/spf13/cobra"
)

const defaultContainerName = "isolarium-container"
const defaultNonoName = "isolarium-nono"
const defaultEC2Name = "isolarium-ec2"

func newCreateCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver) *cobra.Command {
	var workDirFlag string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an isolated environment for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			envType := string(*typeFlag)

			if err := rejectWorkDirectoryForUnsupportedType(cmd, envType); err != nil {
				return err
			}

			name := resolveDefaultName(*nameFlag, envType, rootCmd)

			if envType == "vm" {
				return createAndSetupVM(name)
			}

			b, err := resolver(envType)
			if err != nil {
				return err
			}

			if envType == "ec2" {
				return createAndSetupEC2(b, name, workDirFlag)
			}

			opts := backend.CreateOptions{
				Name:          name,
				WorkDirectory: workDirFlag,
			}
			return b.Create(opts)
		},
	}

	cwd, _ := os.Getwd()
	cmd.Flags().StringVar(&workDirFlag, "work-directory", cwd, "Work directory to mount (container mode only)")

	return cmd
}

func workDirectoryExplicitlySet(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("work-directory")
}

func rejectWorkDirectoryForUnsupportedType(cmd *cobra.Command, envType string) error {
	if !workDirectoryExplicitlySet(cmd) || envType == "container" {
		return nil
	}
	if envType == "vm" {
		return fmt.Errorf("--work-directory is only supported with --type container")
	}
	return fmt.Errorf("--work-directory is not supported with --type %s", envType)
}

var defaultNamesByType = map[string]string{
	"container": defaultContainerName,
	"nono":      defaultNonoName,
	"ec2":       defaultEC2Name,
}

func resolveDefaultName(nameFlag string, envType string, rootCmd *cobra.Command) string {
	if rootCmd.PersistentFlags().Changed("name") {
		return nameFlag
	}
	if defaultName, ok := defaultNamesByType[envType]; ok {
		return defaultName
	}
	return nameFlag
}
