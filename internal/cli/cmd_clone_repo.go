package cli

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/github"
	"github.com/cer/isolarium/internal/lima"
	"github.com/spf13/cobra"
)

func newCloneRepoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone-repo",
		Short: "Clone the repository into the VM (retry after a failed create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			state := lima.GetVMState(vmNameFlag)
			if state == "none" {
				return fmt.Errorf("no VM exists; run 'isolarium create' first")
			}
			if state == "stopped" {
				fmt.Println("Starting stopped VM...")
				if err := lima.StartVM(vmNameFlag); err != nil {
					return err
				}
			}

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

			owner, repo, err := github.ParseRepoURL(remoteURL)
			if err != nil {
				return fmt.Errorf("failed to parse repository URL: %w", err)
			}

			token, err := mintGitHubToken()
			if err != nil {
				return err
			}

			fmt.Println("Cloning repository...")
			if err := lima.CloneRepo(vmNameFlag, cwd, remoteURL, branch, token); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}

			if err := lima.WriteRepoMetadata(vmNameFlag, owner, repo, branch); err != nil {
				return fmt.Errorf("failed to write metadata: %w", err)
			}

			fmt.Println("Repository cloned successfully")
			return nil
		},
	}
}
