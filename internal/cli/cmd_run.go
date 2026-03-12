package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

type runOptions struct {
	name        string
	args        []string
	copySession bool
	freshLogin  bool
	interactive bool
	noGHToken   bool
	readPaths   []string
}

func newRunCmdWithResolver(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, resolver BackendResolver, envTypeResolver EnvironmentTypeResolver) *cobra.Command {
	var opts runOptions

	cmd := &cobra.Command{
		Use:   "run [flags] -- command [args...]",
		Short: "Execute a command inside an isolated environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified; use: isolarium run -- <command> [args...]")
			}

			opts.args = args
			opts.name = resolveDefaultName(*nameFlag, string(*typeFlag), rootCmd)

			envType, err := resolveEnvType(rootCmd, typeFlag, opts.name, envTypeResolver)
			if err != nil {
				return err
			}

			if envType == "vm" {
				return runInVM(opts, cmd)
			}

			if envType == "nono" {
				if cmd.Flags().Changed("copy-session") {
					return fmt.Errorf("--copy-session is not supported with --type nono")
				}
				if cmd.Flags().Changed("fresh-login") {
					return fmt.Errorf("--fresh-login is not supported with --type nono")
				}
				return runInNono(opts, resolver)
			}

			return runInContainer(opts, resolver, envType)
		},
	}

	cmd.Flags().BoolVar(&opts.copySession, "copy-session", true, "Copy Claude credentials from host to VM")
	cmd.Flags().BoolVar(&opts.freshLogin, "fresh-login", false, "Use device code flow for fresh Claude session (disables --copy-session)")
	cmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", false, "Attach TTY for interactive commands")
	cmd.Flags().StringSliceVar(&opts.readPaths, "read", nil, "Grant nono sandbox read-only access to additional paths")
	cmd.Flags().BoolVar(&opts.noGHToken, "no-gh-token", false, "Disable GitHub token minting and GH_TOKEN injection")

	return cmd
}

type tokenFetcher func() (string, error)

func tokenEnvVars(fetch tokenFetcher, buildVars func(string) map[string]string) (map[string]string, error) {
	token, err := fetch()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return map[string]string{}, nil
	}
	return buildVars(token), nil
}

func addTokenEnvVars(envVars map[string]string, noGHToken bool, fetch tokenFetcher, buildVars func(string) map[string]string) error {
	if noGHToken {
		return nil
	}
	tokenVars, err := tokenEnvVars(fetch, buildVars)
	if err != nil {
		return err
	}
	for k, v := range tokenVars {
		envVars[k] = v
	}
	return nil
}

func vmTokenVars(token string) map[string]string {
	return map[string]string{"GIT_TOKEN": token, "GH_TOKEN": token}
}

func containerTokenVars(token string) map[string]string {
	return map[string]string{"GH_TOKEN": token}
}

func nonoTokenVars(token string) map[string]string {
	return map[string]string{
		"GH_TOKEN":           token,
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "url.https://x-access-token:" + token + "@github.com/.insteadOf",
		"GIT_CONFIG_VALUE_0": "git@github.com:",
	}
}

func execBackendCommand(b backend.Backend, opts runOptions, envVars map[string]string) error {
	var exitCode int
	var err error
	if opts.interactive {
		exitCode, err = b.ExecInteractive(opts.name, envVars, opts.args)
	} else {
		exitCode, err = b.Exec(opts.name, envVars, opts.args)
	}
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

func prepareVMSession(opts runOptions, cmd *cobra.Command) error {
	if opts.freshLogin && cmd.Flags().Changed("copy-session") {
		return fmt.Errorf("--fresh-login and --copy-session are mutually exclusive")
	}
	if opts.freshLogin {
		opts.copySession = false
	}
	if err := ensureVMRunning(opts.name); err != nil {
		return err
	}
	if !opts.copySession {
		return nil
	}
	if err := copyClaudeCredentialsToVM(opts.name); err != nil {
		return fmt.Errorf("failed to copy credentials: %w", err)
	}
	return nil
}

// test seam: tests swap this in cmd_run_test.go to stub VM execution
var runInVM = runInVMImpl

func runInVMImpl(opts runOptions, cmd *cobra.Command) error {
	if err := prepareVMSession(opts, cmd); err != nil {
		return err
	}

	envVars, err := buildVMEnvVars(opts.noGHToken)
	if err != nil {
		return err
	}

	return execInVM(opts.name, envVars, opts.args, opts.interactive)
}

func buildVMEnvVars(noGHToken bool) (map[string]string, error) {
	envVars := map[string]string{}
	for k, v := range GetEnvVars() {
		envVars[k] = v
	}
	if err := addTokenEnvVars(envVars, noGHToken, mintGitHubToken, vmTokenVars); err != nil {
		return nil, err
	}
	return envVars, nil
}

func execInVM(name string, envVars map[string]string, args []string, interactive bool) error {
	homeDir, err := lima.GetVMHomeDir(name)
	if err != nil {
		return fmt.Errorf("failed to get VM home directory: %w", err)
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

func runInNono(opts runOptions, resolver BackendResolver) error {
	b, err := resolver("nono")
	if err != nil {
		return err
	}

	if nb, ok := b.(*backend.NonoBackend); ok {
		nb.ExtraReadPaths = opts.readPaths
	}

	envVars := map[string]string{
		"PRE_COMMIT_HOME": filepath.Join(os.TempDir(), "pre-commit"),
		"UV_CACHE_DIR":    filepath.Join(os.TempDir(), "uv-cache"),
		"UV_TOOL_DIR":     filepath.Join(os.TempDir(), "uv-tools"),
	}
	for k, v := range GetEnvVars() {
		envVars[k] = v
	}
	if err := addTokenEnvVars(envVars, opts.noGHToken, mintGitHubToken, nonoTokenVars); err != nil {
		return err
	}

	return execBackendCommand(b, opts, envVars)
}

func runInContainer(opts runOptions, resolver BackendResolver, envType string) error {
	b, err := resolver(envType)
	if err != nil {
		return err
	}

	if opts.copySession {
		credentials, credErr := readKeychainCredentials()
		if credErr != nil {
			return fmt.Errorf("failed to read credentials: %w", credErr)
		}
		if err := b.CopyCredentials(opts.name, credentials); err != nil {
			return fmt.Errorf("failed to copy credentials: %w", err)
		}
	}

	envVars := map[string]string{}
	for k, v := range GetEnvVars() {
		envVars[k] = v
	}
	if err := addTokenEnvVars(envVars, opts.noGHToken, extractGitHubToken, containerTokenVars); err != nil {
		return err
	}

	return execBackendCommand(b, opts, envVars)
}
