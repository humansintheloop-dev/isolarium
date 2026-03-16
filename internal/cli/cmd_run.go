package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/config"
	"github.com/humansintheloop-dev/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

var loadRunEnvVars = loadRunEnvVarsImpl

func loadRunEnvVarsImpl(isolationType string) (map[string]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	cfg, err := config.LoadPidConfig(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to load pid.yaml: %w", err)
	}
	if cfg == nil {
		return map[string]string{}, nil
	}

	var envNames []string
	switch isolationType {
	case "container":
		envNames = cfg.Container.Run.Env
	case "vm":
		envNames = cfg.VM.Run.Env
	case "nono":
		envNames = cfg.Nono.Run.Env
	}

	result := make(map[string]string, len(envNames))
	for _, name := range envNames {
		result[name] = os.Getenv(name)
	}
	return result, nil
}

type runOptions struct {
	name          string
	args          []string
	copySession   bool
	freshLogin    bool
	interactive   bool
	noGHToken     bool
	readPaths     []string
	create        bool
	workDirectory string
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

			if cmd.Flags().Changed("work-directory") && !opts.create {
				return fmt.Errorf("--work-directory requires --create")
			}

			if envType == "vm" {
				return runInVM(opts, cmd)
			}

			if envType == "nono" {
				if err := rejectVMOnlyFlags(cmd); err != nil {
					return err
				}
				return runInNono(opts, resolver)
			}

			return runInContainer(opts, resolver, envType)
		},
	}

	cwd, _ := os.Getwd()
	cmd.Flags().BoolVar(&opts.copySession, "copy-session", true, "Copy Claude credentials from host to VM")
	cmd.Flags().BoolVar(&opts.freshLogin, "fresh-login", false, "Use device code flow for fresh Claude session (disables --copy-session)")
	cmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", false, "Attach TTY for interactive commands")
	cmd.Flags().StringSliceVar(&opts.readPaths, "read", nil, "Grant nono sandbox read-only access to additional paths")
	cmd.Flags().BoolVar(&opts.noGHToken, "no-gh-token", false, "Disable GitHub token minting and GH_TOKEN injection")
	cmd.Flags().BoolVar(&opts.create, "create", false, "Create the environment if it does not exist")
	cmd.Flags().StringVar(&opts.workDirectory, "work-directory", cwd, "Work directory to mount (container mode, requires --create)")

	return cmd
}

func rejectVMOnlyFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("copy-session") {
		return fmt.Errorf("--copy-session is not supported with --type nono")
	}
	if cmd.Flags().Changed("fresh-login") {
		return fmt.Errorf("--fresh-login is not supported with --type nono")
	}
	return nil
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
	req := backend.ExecRequest{
		ContainerName: opts.name,
		EnvVars:       envVars,
		Args:          opts.args,
	}
	var exitCode int
	var err error
	if opts.interactive {
		exitCode, err = b.ExecInteractive(req)
	} else {
		exitCode, err = b.Exec(req)
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

func buildRunEnvVars(isolationType string, baseVars map[string]string, noGHToken bool, fetch tokenFetcher, buildVars func(string) map[string]string) (map[string]string, error) {
	pidEnvVars, err := loadRunEnvVars(isolationType)
	if err != nil {
		return nil, err
	}
	envVars := make(map[string]string, len(baseVars)+len(pidEnvVars))
	for k, v := range baseVars {
		envVars[k] = v
	}
	for k, v := range pidEnvVars {
		envVars[k] = v
	}
	for k, v := range GetEnvVars() {
		envVars[k] = v
	}
	if err := addTokenEnvVars(envVars, noGHToken, fetch, buildVars); err != nil {
		return nil, err
	}
	return envVars, nil
}

func buildVMEnvVars(noGHToken bool) (map[string]string, error) {
	return buildRunEnvVars("vm", nil, noGHToken, mintGitHubToken, vmTokenVars)
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

func buildNonoEnvVars(noGHToken bool) (map[string]string, error) {
	baseVars := map[string]string{
		"PRE_COMMIT_HOME": filepath.Join(os.TempDir(), "pre-commit"),
		"UV_CACHE_DIR":    filepath.Join(os.TempDir(), "uv-cache"),
		"UV_TOOL_DIR":     filepath.Join(os.TempDir(), "uv-tools"),
	}
	return buildRunEnvVars("nono", baseVars, noGHToken, mintGitHubToken, nonoTokenVars)
}

func createIfNeeded(b backend.Backend, opts runOptions) error {
	if !opts.create {
		return nil
	}
	createOpts := backend.CreateOptions{Name: opts.name, WorkDirectory: opts.workDirectory}

	if b.GetState(opts.name) == "none" {
		fmt.Printf("Creating environment (container: %s)...\n", opts.name)
		return b.Create(createOpts)
	}
	db, ok := b.(*backend.DockerBackend)
	if !ok {
		return nil
	}
	if db.WorkDirectoryChanged(opts.name, opts.workDirectory) {
		fmt.Printf("Work directory changed, recreating container %s...\n", opts.name)
		if err := db.Destroy(opts.name); err != nil {
			return err
		}
		return db.Create(createOpts)
	}
	_, err := db.RebuildIfChanged(createOpts)
	return err
}

func runInNono(opts runOptions, resolver BackendResolver) error {
	b, err := resolver("nono")
	if err != nil {
		return err
	}

	if err := createIfNeeded(b, opts); err != nil {
		return err
	}

	if nb, ok := b.(*backend.NonoBackend); ok {
		nb.ExtraReadPaths = opts.readPaths
	}

	envVars, err := buildNonoEnvVars(opts.noGHToken)
	if err != nil {
		return err
	}

	return execBackendCommand(b, opts, envVars)
}

func buildContainerEnvVars(noGHToken bool) (map[string]string, error) {
	return buildRunEnvVars("container", nil, noGHToken, extractGitHubToken, containerTokenVars)
}

func runInContainer(opts runOptions, resolver BackendResolver, envType string) error {
	b, err := resolver(envType)
	if err != nil {
		return err
	}

	if err := createIfNeeded(b, opts); err != nil {
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

	envVars, err := buildContainerEnvVars(opts.noGHToken)
	if err != nil {
		return err
	}

	return execBackendCommand(b, opts, envVars)
}
