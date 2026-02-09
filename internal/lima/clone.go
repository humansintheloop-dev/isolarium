package lima

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BuildCloneURL constructs the git clone URL, embedding token if provided.
// Converts SSH URLs to HTTPS format for token authentication.
func BuildCloneURL(remoteURL, token string) string {
	// Convert SSH to HTTPS if needed
	url := remoteURL
	if strings.HasPrefix(url, "git@github.com:") {
		// git@github.com:owner/repo.git -> https://github.com/owner/repo.git
		path := strings.TrimPrefix(url, "git@github.com:")
		url = "https://github.com/" + path
	}

	// Embed token if provided
	if token != "" {
		// https://github.com/... -> https://x-access-token:TOKEN@github.com/...
		url = strings.Replace(url, "https://github.com/", "https://x-access-token:"+token+"@github.com/", 1)
	}

	return url
}

// BuildCloneCommand constructs the limactl command to clone a repo inside the VM
func BuildCloneCommand(cloneURL, branch string) []string {
	return []string{
		"limactl", "shell", vmName, "--",
		"git", "clone", "--branch", branch, cloneURL, "repo",
	}
}

const workflowToolsRepo = "https://github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git"

// BuildWorkflowToolsCloneCommand constructs the limactl command to clone workflow tools into the VM
func BuildWorkflowToolsCloneCommand(token string) []string {
	cloneURL := workflowToolsRepo
	if token != "" {
		cloneURL = strings.Replace(cloneURL, "https://github.com/", "https://x-access-token:"+token+"@github.com/", 1)
	}
	return []string{
		"limactl", "shell", vmName, "--",
		"git", "clone", cloneURL, "workflow-tools",
	}
}

// CloneRepo clones a repository inside the Lima VM
func CloneRepo(remoteURL, branch, token string) error {
	cloneURL := BuildCloneURL(remoteURL, token)
	args := BuildCloneCommand(cloneURL, branch)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

// CloneWorkflowTools clones the workflow tools repository into the VM
func CloneWorkflowTools(token string) error {
	args := BuildWorkflowToolsCloneCommand(token)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone workflow tools: %w", err)
	}

	return nil
}

// BuildInstallMarketplaceCommand constructs the command to run install-marketplace.sh
func BuildInstallMarketplaceCommand() []string {
	return []string{
		"limactl", "shell", vmName, "--",
		"bash", "-c", "cd ~/workflow-tools && ./install-marketplace.sh",
	}
}

// InstallMarketplacePlugins runs the install-marketplace.sh script
func InstallMarketplacePlugins() error {
	args := BuildInstallMarketplaceCommand()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install marketplace plugins: %w", err)
	}

	return nil
}

// BuildInstallPluginCommand constructs the command to run install-plugin.sh
func BuildInstallPluginCommand() []string {
	return []string{
		"limactl", "shell", vmName, "--",
		"bash", "-c", "cd ~/workflow-tools && ./install-plugin.sh",
	}
}

// InstallPlugins runs the install-plugin.sh script
func InstallPlugins() error {
	args := BuildInstallPluginCommand()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install plugins: %w", err)
	}

	return nil
}
