package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cer/isolarium/internal/backend"
	"github.com/cer/isolarium/internal/claude"
	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/github"
	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

var vmNameFlag string
var envTypeFlag = environmentType("vm")

// BackendResolver resolves a Backend from an environment type string.
type BackendResolver func(envType string) (backend.Backend, error)

func NewRootCmd() *cobra.Command {
	return newRootCmdWithResolver(backend.ResolveBackend)
}

func newRootCmdWithResolver(resolver BackendResolver) *cobra.Command {
	var nameFlag string
	var typeFlag environmentType = "vm"

	rootCmd := &cobra.Command{
		Use:   "isolarium",
		Short: "Secure execution environment for coding agents",
	}

	rootCmd.PersistentFlags().StringVar(&nameFlag, "name", lima.GetVMName(), "Name of the environment")
	rootCmd.PersistentFlags().Var(&typeFlag, "type", `Environment type: "vm" or "container" (default "vm")`)

	rootCmd.AddCommand(newCreateCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver))
	rootCmd.AddCommand(newDestroyCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver))
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newRunCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver))
	rootCmd.AddCommand(newShellCmdWithResolver(rootCmd, &nameFlag, &typeFlag, resolver))
	rootCmd.AddCommand(newSshCmd())
	rootCmd.AddCommand(newCloneRepoCmd())
	rootCmd.AddCommand(newInstallToolsCmd())
	rootCmd.AddCommand(newInstallWorkflowToolsFromSourceCmd())

	return rootCmd
}

func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return nil // File doesn't exist, skip silently
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Validate that _PATH variables reference existing files
			if strings.HasSuffix(key, "_PATH") && value != "" {
				if _, err := os.Stat(value); os.IsNotExist(err) {
					return fmt.Errorf("%s references non-existent file: %s", key, value)
				}
			}

			// Only set if not already set in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
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

func mintGitHubToken() (string, error) {
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
