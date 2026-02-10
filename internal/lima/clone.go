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

func BuildCloneCommand(name, cloneURL, branch string) []string {
	return []string{
		"limactl", "shell", name, "--",
		"git", "clone", "--branch", branch, cloneURL, "repo",
	}
}

const workflowToolsRepo = "https://github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git"

func BuildWorkflowToolsCloneCommand(name, token string) []string {
	cloneURL := workflowToolsRepo
	if token != "" {
		cloneURL = strings.Replace(cloneURL, "https://github.com/", "https://x-access-token:"+token+"@github.com/", 1)
	}
	return []string{
		"limactl", "shell", name, "--",
		"git", "clone", cloneURL, "workflow-tools",
	}
}

func CloneRepo(name, remoteURL, branch, token string) error {
	cloneURL := BuildCloneURL(remoteURL, token)
	args := BuildCloneCommand(name, cloneURL, branch)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

func CloneWorkflowTools(name, token string) error {
	args := BuildWorkflowToolsCloneCommand(name, token)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone workflow tools: %w", err)
	}

	return nil
}

func BuildInstallPluginCommand(name string) []string {
	return []string{
		"limactl", "shell", name, "--",
		"bash", "-c", "cd ~/workflow-tools && ./install-plugin.sh",
	}
}

func InstallPlugins(name string) error {
	args := BuildInstallPluginCommand(name)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install plugins: %w", err)
	}

	return nil
}

func BuildInstallI2CodeCommand(name string) []string {
	return []string{
		"limactl", "shell", name, "--",
		"bash", "-lc", "cd ~/workflow-tools && uv tool install -e .",
	}
}

func InstallI2Code(name string) error {
	args := BuildInstallI2CodeCommand(name)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install i2code CLI: %w", err)
	}

	return nil
}
