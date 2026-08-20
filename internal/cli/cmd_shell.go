package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/spf13/cobra"
)

func newShellCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver, envTypeResolver EnvironmentTypeResolver) *cobra.Command {
	var copySession bool
	var newSession bool

	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open an interactive shell inside the environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := resolveDefaultName(*nameFlag, string(*typeFlag), rootCmd)

			envType, err := resolveEnvType(rootCmd, typeFlag, name, envTypeResolver)
			if err != nil {
				return err
			}

			if err := rejectFlagsUnsupportedByShell(cmd, envType, newSession); err != nil {
				return err
			}

			b, err := resolver(envType)
			if err != nil {
				return err
			}
			applyNewSession(b, newSession)

			if err := copyCredentialsForContainerShell(b, envType, name, copySession); err != nil {
				return err
			}

			envVars, err := buildShellEnvVars(envType)
			if err != nil {
				return err
			}

			exitCode, execErr := b.OpenShell(backend.ExecRequest{ContainerName: name, EnvVars: envVars})
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
	cmd.Flags().BoolVar(&newSession, "new-session", false, newSessionFlagUsage)

	return cmd
}

func rejectFlagsUnsupportedByShell(cmd *cobra.Command, envType string, newSession bool) error {
	if envType == "nono" && cmd.Flags().Changed("copy-session") {
		return fmt.Errorf("--copy-session is not supported with --type nono")
	}
	return rejectNewSessionOutsideEC2(envType, newSession)
}

// copyCredentialsForContainerShell carries the host's Claude credentials into a
// container before its shell opens. Every other environment type opens straight
// into its shell: vm and nono never copy, and ec2 leaves the decision to the
// backend, which only overwrites a credential file the host's is fresher than.
func copyCredentialsForContainerShell(b backend.Backend, envType, name string, copySession bool) error {
	if !copySession || envType != "container" {
		return nil
	}

	credentials, err := readKeychainCredentials()
	if err != nil {
		return fmt.Errorf("failed to read credentials: %w", err)
	}
	if err := b.CopyCredentials(name, credentials); err != nil {
		return fmt.Errorf("failed to copy credentials: %w", err)
	}
	return nil
}

func buildShellEnvVars(envType string) (map[string]string, error) {
	envVars := map[string]string{}
	for k, v := range GetEnvVars() {
		envVars[k] = v
	}

	if envType == "nono" {
		envVars["PRE_COMMIT_HOME"] = filepath.Join(os.TempDir(), "pre-commit")
		envVars["UV_CACHE_DIR"] = filepath.Join(os.TempDir(), "uv-cache")
		envVars["UV_TOOL_DIR"] = filepath.Join(os.TempDir(), "uv-tools")
	}

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
