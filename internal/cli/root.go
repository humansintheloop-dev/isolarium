package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/github"
	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "isolarium",
		Short: "Secure execution environment for coding agents",
	}

	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newCreateCmd())
	rootCmd.AddCommand(newDestroyCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newSshCmd())

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

func copyClaudeCredentialsToVM() error {
	credentialsPath := os.Getenv("CLAUDE_CREDENTIALS_PATH")
	if credentialsPath == "" {
		return fmt.Errorf("CLAUDE_CREDENTIALS_PATH environment variable not set")
	}
	fmt.Println("Copying Claude credentials to VM...")
	return lima.CopyClaudeCredentials(credentialsPath)
}

// mintGitHubToken mints a GitHub App installation token if the app is configured.
// Uses the host's git remote to determine owner/repo.
// Returns empty string if GitHub App is not configured.
func mintGitHubToken() (string, error) {
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if appID == "" || privateKeyPath == "" {
		return "", nil
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
