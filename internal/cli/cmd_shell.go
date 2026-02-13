package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newShellCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver, envTypeResolver EnvironmentTypeResolver) *cobra.Command {
	var copySession bool

	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open an interactive shell inside the environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := resolveDefaultName(*nameFlag, string(*typeFlag), rootCmd)

			envType, err := resolveEnvType(rootCmd, typeFlag, name, envTypeResolver)
			if err != nil {
				return err
			}

			b, err := resolver(envType)
			if err != nil {
				return err
			}

			if copySession && envType == "container" {
				credentials, credErr := readKeychainCredentials()
				if credErr != nil {
					return fmt.Errorf("failed to read credentials: %w", credErr)
				}
				if err := b.CopyCredentials(name, credentials); err != nil {
					return fmt.Errorf("failed to copy credentials: %w", err)
				}
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

	cmd.Flags().BoolVar(&copySession, "copy-session", true, "Copy Claude credentials from host to container")

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
