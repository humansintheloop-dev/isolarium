package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newRunCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver, envTypeResolver EnvironmentTypeResolver) *cobra.Command {
	var copySession bool
	var freshLogin bool
	var interactive bool

	cmd := &cobra.Command{
		Use:   "run [flags] -- command [args...]",
		Short: "Execute a command inside an isolated environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified; use: isolarium run -- <command> [args...]")
			}

			name := resolveDefaultName(*nameFlag, string(*typeFlag), rootCmd)

			envType, err := resolveEnvType(rootCmd, typeFlag, name, envTypeResolver)
			if err != nil {
				return err
			}

			if envType == "vm" {
				return runInVM(name, args, copySession, freshLogin, interactive, cmd)
			}

			if envType == "nono" {
				if cmd.Flags().Changed("copy-session") {
					return fmt.Errorf("--copy-session is not supported with --type nono")
				}
				if cmd.Flags().Changed("fresh-login") {
					return fmt.Errorf("--fresh-login is not supported with --type nono")
				}
				return runInNono(name, args, interactive, resolver)
			}

			return runInContainer(name, args, copySession, interactive, resolver, envType)
		},
	}

	cmd.Flags().BoolVar(&copySession, "copy-session", true, "Copy Claude credentials from host to VM")
	cmd.Flags().BoolVar(&freshLogin, "fresh-login", false, "Use device code flow for fresh Claude session (disables --copy-session)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Attach TTY for interactive commands")

	return cmd
}

func runInVM(name string, args []string, copySession bool, freshLogin bool, interactive bool, cmd *cobra.Command) error {
	if freshLogin && cmd.Flags().Changed("copy-session") {
		return fmt.Errorf("--fresh-login and --copy-session are mutually exclusive")
	}

	if freshLogin {
		copySession = false
	}

	if err := ensureVMRunning(name); err != nil {
		return err
	}

	if copySession {
		if err := copyClaudeCredentialsToVM(name); err != nil {
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

	homeDir, homeErr := lima.GetVMHomeDir(name)
	if homeErr != nil {
		return fmt.Errorf("failed to get VM home directory: %w", homeErr)
	}
	workdir := homeDir + "/repo"

	var exitCode int
	var execErr error
	if interactive {
		exitCode, execErr = lima.ExecInteractiveCommand(name, workdir, envVars, args)
	} else {
		exitCode, execErr = lima.ExecCommand(name, workdir, envVars, args)
	}
	if execErr != nil {
		return fmt.Errorf("failed to execute command: %w", execErr)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

func runInNono(name string, args []string, interactive bool, resolver BackendResolver) error {
	b, err := resolver("nono")
	if err != nil {
		return err
	}

	envVars := map[string]string{
		"PRE_COMMIT_HOME": filepath.Join(os.TempDir(), "pre-commit"),
		"UV_CACHE_DIR":    filepath.Join(os.TempDir(), "uv-cache"),
	}
	token, tokenErr := mintGitHubToken()
	if tokenErr != nil {
		return tokenErr
	}
	if token != "" {
		envVars["GH_TOKEN"] = token
		envVars["GIT_CONFIG_COUNT"] = "1"
		envVars["GIT_CONFIG_KEY_0"] = "url.https://x-access-token:" + token + "@github.com/.insteadOf"
		envVars["GIT_CONFIG_VALUE_0"] = "git@github.com:"
	}

	var exitCode int
	var execErr error
	if interactive {
		exitCode, execErr = b.ExecInteractive(name, envVars, args)
	} else {
		exitCode, execErr = b.Exec(name, envVars, args)
	}
	if execErr != nil {
		return fmt.Errorf("failed to execute command: %w", execErr)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

func runInContainer(name string, args []string, copySession bool, interactive bool, resolver BackendResolver, envType string) error {
	b, err := resolver(envType)
	if err != nil {
		return err
	}

	if copySession {
		credentials, credErr := readKeychainCredentials()
		if credErr != nil {
			return fmt.Errorf("failed to read credentials: %w", credErr)
		}
		if err := b.CopyCredentials(name, credentials); err != nil {
			return fmt.Errorf("failed to copy credentials: %w", err)
		}
	}

	envVars := map[string]string{}
	token, tokenErr := extractGitHubToken()
	if tokenErr != nil {
		return tokenErr
	}
	if token != "" {
		envVars["GH_TOKEN"] = token
	}

	var exitCode int
	var execErr error
	if interactive {
		exitCode, execErr = b.ExecInteractive(name, envVars, args)
	} else {
		exitCode, execErr = b.Exec(name, envVars, args)
	}
	if execErr != nil {
		return fmt.Errorf("failed to execute command: %w", execErr)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
