package cli

import (
	"fmt"
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/git"
	"github.com/humansintheloop-dev/isolarium/internal/github"
	"github.com/humansintheloop-dev/isolarium/internal/lima"
)

func ensureVMRunning(name string) error {
	state := lima.GetVMState(name)
	switch state {
	case "running":
		return nil
	case "stopped":
		fmt.Println("Starting stopped VM...")
		return lima.StartVM(name)
	default:
		return createAndSetupVM(name)
	}
}

func createAndSetupVM(name string) error {
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
	if err := lima.CreateVM(name); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
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
	if err := lima.CloneRepo(name, cwd, remoteURL, branch, token); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	if err := lima.WriteRepoMetadata(name, owner, repo, branch); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	fmt.Println("Installing Java and Gradle via SDKMAN...")
	if err := lima.InstallUsingSDKMAN(name); err != nil {
		return fmt.Errorf("failed to install Java/Gradle: %w", err)
	}

	if err := installWorkflowTools(name); err != nil {
		return err
	}

	fmt.Println("VM created successfully")
	return nil
}

func installWorkflowTools(name string) error {
	fmt.Println("Cloning workflow tools...")
	if err := lima.CloneWorkflowTools(name, ""); err != nil {
		return fmt.Errorf("failed to clone workflow tools: %w", err)
	}

	fmt.Println("Installing custom plugins...")
	if err := lima.InstallPlugins(name); err != nil {
		return fmt.Errorf("failed to install custom plugins: %w", err)
	}

	fmt.Println("Installing i2code CLI...")
	if err := lima.InstallI2Code(name); err != nil {
		return fmt.Errorf("failed to install i2code CLI: %w", err)
	}

	return nil
}
