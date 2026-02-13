package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newShellCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open an interactive shell inside the environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			envType := string(*typeFlag)
			name := resolveDefaultName(*nameFlag, envType, rootCmd)

			b, err := resolver(envType)
			if err != nil {
				return err
			}

			envVars, err := buildShellEnvVars(envType)
			if err != nil {
				return err
			}

			exitCode, execErr := b.OpenShell(name, envVars)
			if execErr != nil {
				return fmt.Errorf("failed to open shell: %w", execErr)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}

			return nil
		},
	}

	return cmd
}

func buildShellEnvVars(envType string) (map[string]string, error) {
	envVars := map[string]string{}

	if envType == "container" {
		token, err := extractGitHubToken()
		if err != nil {
			return nil, err
		}
		if token != "" {
			envVars["GH_TOKEN"] = token
		}
	}

	return envVars, nil
}
