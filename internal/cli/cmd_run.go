package cli

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var copySession bool
	var freshLogin bool
	var interactive bool

	cmd := &cobra.Command{
		Use:   "run [flags] -- command [args...]",
		Short: "Execute a command inside the VM in the repo directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified; use: isolarium run -- <command> [args...]")
			}

			if freshLogin && cmd.Flags().Changed("copy-session") {
				return fmt.Errorf("--fresh-login and --copy-session are mutually exclusive")
			}

			if freshLogin {
				copySession = false
			}

			exists, err := lima.VMExists()
			if err != nil {
				return fmt.Errorf("failed to check VM status: %w", err)
			}
			if !exists {
				return fmt.Errorf("no VM exists; run 'isolarium create' first")
			}

			if copySession {
				if err := copyClaudeCredentialsToVM(); err != nil {
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

			homeDir, homeErr := lima.GetVMHomeDir()
			if homeErr != nil {
				return fmt.Errorf("failed to get VM home directory: %w", homeErr)
			}
			workdir := homeDir + "/repo"

			var exitCode int
			if interactive {
				exitCode, err = lima.ExecInteractiveCommand(lima.GetVMName(), workdir, envVars, args)
			} else {
				exitCode, err = lima.ExecCommand(lima.GetVMName(), workdir, envVars, args)
			}
			if err != nil {
				return fmt.Errorf("failed to execute command: %w", err)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&copySession, "copy-session", true, "Copy Claude credentials from host to VM")
	cmd.Flags().BoolVar(&freshLogin, "fresh-login", false, "Use device code flow for fresh Claude session (disables --copy-session)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Attach TTY for interactive commands")

	return cmd
}
