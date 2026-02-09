package cli

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/github"
	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create and start a Lima VM for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			remoteURL, err := git.GetRemoteURL(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository (or no remote configured): %w", err)
			}

			branch, err := git.GetCurrentBranch(cwd)
			if err != nil {
				return fmt.Errorf("failed to get current branch: %w", err)
			}

			fmt.Printf("Repository: %s\n", remoteURL)
			fmt.Printf("Branch: %s\n", branch)

			fmt.Println("Creating Lima VM...")
			if err := lima.CreateVM(); err != nil {
				return fmt.Errorf("failed to create VM: %w", err)
			}

			owner, repo, err := github.ParseRepoURL(remoteURL)
			if err != nil {
				return fmt.Errorf("failed to parse repository URL: %w", err)
			}

			var token string
			appID := os.Getenv("GITHUB_APP_ID")
			privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
			if appID != "" && privateKeyPath != "" {
				privateKeyBytes, err := os.ReadFile(privateKeyPath)
				if err != nil {
					return fmt.Errorf("failed to read private key from %s: %w", privateKeyPath, err)
				}
				fmt.Println("Minting GitHub App token...")
				minter, err := github.NewTokenMinter(appID, string(privateKeyBytes), "")
				if err != nil {
					return fmt.Errorf("failed to create token minter: %w", err)
				}
				token, err = minter.MintInstallationToken(owner, repo)
				if err != nil {
					return fmt.Errorf("failed to mint token: %w", err)
				}
			}

			fmt.Println("Cloning repository...")
			if err := lima.CloneRepo(remoteURL, branch, token); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}

			if err := lima.WriteRepoMetadata(owner, repo, branch); err != nil {
				return fmt.Errorf("failed to write metadata: %w", err)
			}

			fmt.Println("Cloning workflow tools...")
			if err := lima.CloneWorkflowTools(""); err != nil {
				return fmt.Errorf("failed to clone workflow tools: %w", err)
			}

			fmt.Println("Installing custom plugins...")
			if err := lima.InstallPlugins(); err != nil {
				return fmt.Errorf("failed to install custom plugins: %w", err)
			}

			fmt.Println("Installing i2code CLI...")
			if err := lima.InstallI2Code(); err != nil {
				return fmt.Errorf("failed to install i2code CLI: %w", err)
			}

			fmt.Println("VM created successfully")
			return nil
		},
	}
}
