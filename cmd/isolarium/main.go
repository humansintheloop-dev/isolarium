package main

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/lima"
	"github.com/cer/isolarium/internal/status"
	"github.com/spf13/cobra"
)

func main() {
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
