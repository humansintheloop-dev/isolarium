package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/claude"
	"github.com/humansintheloop-dev/isolarium/internal/git"
	"github.com/humansintheloop-dev/isolarium/internal/github"
	"github.com/humansintheloop-dev/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

var vmNameFlag string

var parsedEnvVars map[string]string

func GetEnvVars() map[string]string {
	if parsedEnvVars == nil {
		return map[string]string{}
	}
	return parsedEnvVars
}

func parseEnvFlags(envFlags []string) map[string]string {
	result := make(map[string]string, len(envFlags))
	for _, entry := range envFlags {
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			result[entry[:idx]] = entry[idx+1:]
		} else {
			result[entry] = os.Getenv(entry)
		}
	}
	return result
}

// BackendResolver resolves a Backend from an environment type string.
type BackendResolver func(envType string) (backend.Backend, error)

// EnvironmentTypeResolver auto-detects the environment type for a given name
// by scanning metadata directories.
type EnvironmentTypeResolver func(name string) (string, error)

func NewRootCmd() *cobra.Command {
	return newRootCmdWithResolvers(backend.ResolveBackend, defaultEnvironmentTypeResolver())
}

func newRootCmdWithResolver(resolver BackendResolver) *cobra.Command {
	return newRootCmdWithResolvers(resolver, nil)
}

func newRootCmdWithResolvers(resolver BackendResolver, envTypeResolver EnvironmentTypeResolver) *cobra.Command {
	var nameFlag string
	var typeFlag environmentType = "vm"
	var envFileFlag string
	var envFlags []string

	rootCmd := &cobra.Command{
		Use:   "isolarium",
		Short: "Secure execution environment for coding agents",
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := LoadEnvFile(envFileFlag); err != nil {
			return err
		}
		parsedEnvVars = parseEnvFlags(envFlags)
		return applyEnvVarDefaults(rootCmd, &nameFlag, &typeFlag)
	}

	rootCmd.PersistentFlags().StringVar(&envFileFlag, "env-file", ".env.local", "Path to environment file")
	rootCmd.PersistentFlags().StringVar(&nameFlag, "name", lima.GetVMName(), "Name of the environment")
	rootCmd.PersistentFlags().Var(&typeFlag, "type", `Environment type: "vm", "container", or "nono" (default "vm")`)
	rootCmd.PersistentFlags().StringSliceVar(&envFlags, "env", nil, "Environment variable: VAR (read from env) or VAR=VALUE (literal)")

	lister := newDefaultEnvironmentLister(resolver)
	rootCmd.AddCommand(newCreateCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver))
	rootCmd.AddCommand(newDestroyCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver, envTypeResolver))
	rootCmd.AddCommand(newStatusCmdWithLister(rootCmd, &nameFlag, &typeFlag, lister))
	rootCmd.AddCommand(newRunCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver, envTypeResolver))
	rootCmd.AddCommand(newShellCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver, envTypeResolver))
	rootCmd.AddCommand(newCloneRepoCmd(rootCmd, &typeFlag))
	rootCmd.AddCommand(newInstallToolsCmd(rootCmd, &typeFlag))
	rootCmd.AddCommand(newInstallWorkflowToolsFromSourceCmd(rootCmd, &typeFlag))

	return rootCmd
}

func newRootCmdWithStatusLister(lister EnvironmentLister) *cobra.Command {
	var nameFlag string
	var typeFlag environmentType = "vm"

	rootCmd := &cobra.Command{
		Use:   "isolarium",
		Short: "Secure execution environment for coding agents",
	}

	rootCmd.PersistentFlags().StringVar(&nameFlag, "name", lima.GetVMName(), "Name of the environment")
	rootCmd.PersistentFlags().Var(&typeFlag, "type", `Environment type: "vm", "container", or "nono" (default "vm")`)

	rootCmd.AddCommand(newStatusCmdWithLister(rootCmd, &nameFlag, &typeFlag, lister))

	return rootCmd
}

func applyEnvVarDefaults(cmd *cobra.Command, nameFlag *string, typeFlag *environmentType) error {
	if !cmd.PersistentFlags().Changed("name") {
		if envName := os.Getenv("ISOLARIUM_NAME"); envName != "" {
			*nameFlag = envName
		}
	}
	if cmd.PersistentFlags().Changed("type") {
		return nil
	}
	envType := os.Getenv("ISOLARIUM_TYPE")
	if envType == "" {
		return nil
	}
	if err := typeFlag.Set(envType); err != nil {
		return fmt.Errorf("ISOLARIUM_TYPE: %w", err)
	}
	return nil
}

func defaultEnvironmentTypeResolver() EnvironmentTypeResolver {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	baseDir := filepath.Join(home, ".isolarium")
	return func(name string) (string, error) {
		return backend.ResolveEnvironmentType(baseDir, name)
	}
}

func resolveEnvType(rootCmd *cobra.Command, typeFlag *environmentType, name string, envTypeResolver EnvironmentTypeResolver) (string, error) {
	if rootCmd.PersistentFlags().Changed("type") || envTypeResolver == nil {
		return string(*typeFlag), nil
	}
	resolved, err := envTypeResolver(name)
	if err != nil {
		if errors.Is(err, backend.ErrNoEnvironmentFound) {
			return string(*typeFlag), nil
		}
		return "", err
	}
	return resolved, nil
}

func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		entry := envEntry{key: strings.TrimSpace(parts[0]), value: strings.TrimSpace(parts[1])}
		if err := entry.applyToEnvironment(); err != nil {
			return err
		}
	}
	return nil
}

type envEntry struct {
	key, value string
}

func (e envEntry) applyToEnvironment() error {
	if err := e.validatePathReference(); err != nil {
		return err
	}
	if os.Getenv(e.key) == "" {
		_ = os.Setenv(e.key, e.value)
	}
	return nil
}

func (e envEntry) validatePathReference() error {
	if !strings.HasSuffix(e.key, "_PATH") || e.value == "" {
		return nil
	}
	if _, err := os.Stat(e.value); os.IsNotExist(err) {
		return fmt.Errorf("%s references non-existent file: %s", e.key, e.value)
	}
	return nil
}

func copyClaudeCredentialsToVM(name string) error {
	credentials, err := claude.ReadCredentialsFromKeychain()
	if err != nil {
		return err
	}
	fmt.Println("Copying Claude credentials to VM...")
	return lima.CopyClaudeCredentials(name, credentials)
}

func extractGitHubToken() (string, error) {
	out, err := execCommandOutput("gh", "auth", "token")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

var execCommandOutput = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var readKeychainCredentials = func() (string, error) {
	return claude.ReadCredentialsFromKeychain()
}

var mintGitHubToken = func() (string, error) {
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if appID == "" || privateKeyPath == "" {
		return "", fmt.Errorf("GitHub App not configured (GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY_PATH must be set, usually via .env.local)")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}
	remoteURL, err := git.GetRemoteURL(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to get git remote URL: %w", err)
	}
	owner, repo, err := github.ParseRepoURL(remoteURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse repository URL: %w", err)
	}
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key from %s: %w", privateKeyPath, err)
	}
	fmt.Println("Minting fresh GitHub App token...")
	minter, err := github.NewTokenMinter(appID, string(privateKeyBytes), "")
	if err != nil {
		return "", fmt.Errorf("failed to create token minter: %w", err)
	}
	token, err := minter.MintInstallationToken(owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to mint token: %w", err)
	}
	return token, nil
}
