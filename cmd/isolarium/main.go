package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/github"
	"github.com/cer/isolarium/internal/lima"
	"github.com/cer/isolarium/internal/status"
	"github.com/spf13/cobra"
)

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return // File doesn't exist, skip silently
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
			// Only set if not already set in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}

func main() {
	// Load .env.local if it exists
	loadEnvFile(".env.local")
	rootCmd := &cobra.Command{
		Use:   "isolarium",
		Short: "Secure execution environment for coding agents",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current status of isolarium environment",
		Run: func(cmd *cobra.Command, args []string) {
			s := status.GetStatus()
			fmt.Printf("VM: %s\n", s.VMState)
			if s.Repository != "" {
				fmt.Printf("Repository: %s\n", s.Repository)
				fmt.Printf("Branch: %s\n", s.Branch)
			}
			if s.GitHubAppConfigured {
				fmt.Println("GitHub App: configured")
			} else {
				fmt.Println("GitHub App: not configured")
			}
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create and start a Lima VM for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current working directory
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			// Check if we're in a git repository by trying to get the remote URL
			remoteURL, err := git.GetRemoteURL(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository (or no remote configured): %w", err)
			}

			// Get current branch
			branch, err := git.GetCurrentBranch(cwd)
			if err != nil {
				return fmt.Errorf("failed to get current branch: %w", err)
			}

			fmt.Printf("Repository: %s\n", remoteURL)
			fmt.Printf("Branch: %s\n", branch)

			// Create the VM
			fmt.Println("Creating Lima VM...")
			if err := lima.CreateVM(); err != nil {
				return fmt.Errorf("failed to create VM: %w", err)
			}

			// Parse owner/repo from URL
			owner, repo, err := github.ParseRepoURL(remoteURL)
			if err != nil {
				return fmt.Errorf("failed to parse repository URL: %w", err)
			}

			// Try to mint a token if GitHub App is configured
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

			// Clone the repository
			fmt.Println("Cloning repository...")
			if err := lima.CloneRepo(remoteURL, branch, token); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}

			// Write metadata
			if err := lima.WriteRepoMetadata(owner, repo, branch); err != nil {
				return fmt.Errorf("failed to write metadata: %w", err)
			}

			fmt.Println("VM created successfully")
			return nil
		},
	}

	destroyCmd := &cobra.Command{
		Use:   "destroy",
		Short: "Delete the Lima VM and all its contents",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Destroying Lima VM...")
			if err := lima.DestroyVM(); err != nil {
				return fmt.Errorf("failed to destroy VM: %w", err)
			}
			fmt.Println("VM destroyed successfully")
			return nil
		},
	}

	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(destroyCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
