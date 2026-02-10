package cli

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newSshCmd() *cobra.Command {
	var copySession bool

	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Open an interactive shell inside the VM",
		RunE: func(cmd *cobra.Command, args []string) error {
			exists, err := lima.VMExists(vmNameFlag)
			if err != nil {
				return fmt.Errorf("failed to check VM status: %w", err)
			}
			if !exists {
				return fmt.Errorf("no VM exists; run 'isolarium create' first")
			}

			if copySession {
				if err := copyClaudeCredentialsToVM(vmNameFlag); err != nil {
					return fmt.Errorf("failed to copy credentials: %w", err)
				}
			}

			envVars := map[string]string{}
			token, tokenErr := mintGitHubToken()
			if tokenErr != nil {
				return tokenErr
			}
			if token != "" {
				envVars["GIT_TOKEN"] = token
				envVars["GH_TOKEN"] = token
			}

			homeDir, homeErr := lima.GetVMHomeDir(vmNameFlag)
			if homeErr != nil {
				return fmt.Errorf("failed to get VM home directory: %w", homeErr)
			}
			workdir := homeDir + "/repo"

			exitCode, err := lima.OpenShell(vmNameFlag, workdir, envVars)
			if err != nil {
				return fmt.Errorf("failed to open shell: %w", err)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&copySession, "copy-session", true, "Copy Claude credentials from host to VM")

	return cmd
}
