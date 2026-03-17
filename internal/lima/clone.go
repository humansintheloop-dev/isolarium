package lima

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/project"
)

//go:embed install-using-sdkman.sh
var installUsingSDKMANScript string

// BuildCloneURL constructs the git clone URL, embedding token if provided.
// Only converts SSH URLs to HTTPS when a token is available for authentication.
func BuildCloneURL(remoteURL, token string) string {
	if token == "" {
		return remoteURL
	}

	url := remoteURL
	if strings.HasPrefix(url, "git@github.com:") {
		path := strings.TrimPrefix(url, "git@github.com:")
		url = "https://github.com/" + path
	}

	url = strings.Replace(url, "https://github.com/", "https://x-access-token:"+token+"@github.com/", 1)
	return url
}

func BuildCloneCommand(name, cloneURL, branch string) []string {
	return []string{
		"limactl", "shell", name, "--",
		"git", "clone", "--branch", branch, cloneURL, "repo",
	}
}

var workflowToolsRepo = "https://github.com/" + project.WorkflowToolsOrgRepo + ".git"

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

var projectConfigFiles = []string{
	".claude/settings.local.json",
	"CLAUDE.md",
}

func CloneRepo(name, hostProjectDir, remoteURL, branch, token string) error {
	cloneURL := BuildCloneURL(remoteURL, token)
	args := BuildCloneCommand(name, cloneURL, branch)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	homeDir, err := GetVMHomeDir(name)
	if err != nil {
		return fmt.Errorf("failed to get VM home directory: %w", err)
	}
	repoDir := homeDir + "/repo"

	for _, f := range projectConfigFiles {
		src := hostProjectDir + "/" + f
		if _, err := os.Stat(src); err != nil {
			continue
		}
		fmt.Printf("Copying %s to VM...\n", f)
		if err := CopyFileToVM(name, src, repoDir+"/"+f); err != nil {
			return fmt.Errorf("failed to copy %s to VM: %w", f, err)
		}
	}

	return nil
}

func CloneWorkflowTools(name, token string) error {
	return runCommand(BuildWorkflowToolsCloneCommand(name, token), "clone workflow tools")
}

func BuildConfigureGitAuthorCommand(name, email, userName string) []string {
	return []string{
		"limactl", "shell", name, "--",
		"bash", "-c", "cd ~/repo && git config user.email '" + email + "' && git config user.name '" + userName + "'",
	}
}

func ConfigureGitAuthor(name, email, userName string) error {
	return runCommand(BuildConfigureGitAuthorCommand(name, email, userName), "configure git author")
}

func BuildInstallPluginCommand(name string) []string {
	return []string{
		"limactl", "shell", name, "--",
		"bash", "-c", "cd ~/workflow-tools && ./install-plugin.sh",
	}
}

func InstallPlugins(name string) error {
	return runCommand(BuildInstallPluginCommand(name), "install plugins")
}

func UninstallI2Code(name string) error {
	cmd := exec.Command("limactl", "shell", name, "--",
		"bash", "-lc", "uv tool uninstall i2code")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // best-effort; may not be installed
	return nil
}

func CopyDirToVM(name, localPath, remotePath string) error {
	// Use git ls-files to list tracked + untracked files, respecting .gitignore,
	// then tar them up and extract in the VM.
	tar := exec.Command("bash", "-c",
		"cd "+localPath+" && git ls-files -co --exclude-standard -z | xargs -0 ls -d 2>/dev/null | COPYFILE_DISABLE=1 tar --no-mac-metadata -cf - -T -")
	untar := exec.Command("limactl", "shell", name, "--",
		"bash", "-c", "rm -rf "+remotePath+" && mkdir -p "+remotePath+" && tar -C "+remotePath+" -xf - 2>/dev/null")

	pipe, err := tar.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}
	untar.Stdin = pipe
	untar.Stdout = os.Stdout
	untar.Stderr = os.Stderr

	if err := tar.Start(); err != nil {
		return fmt.Errorf("failed to start tar: %w", err)
	}
	if err := untar.Start(); err != nil {
		return fmt.Errorf("failed to start untar in VM: %w", err)
	}

	if err := tar.Wait(); err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}
	if err := untar.Wait(); err != nil {
		return fmt.Errorf("untar in VM failed: %w", err)
	}

	return nil
}

func CopyFileToVM(name, localPath, remotePath string) error {
	content, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists, then write file
	dir := remotePath[:strings.LastIndex(remotePath, "/")]
	cmd := exec.Command("limactl", "shell", name, "--",
		"bash", "-c", "mkdir -p "+dir+" && cat > "+remotePath)
	cmd.Stdin = strings.NewReader(string(content))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy file to VM: %w\noutput: %s", err, output)
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
	return runCommand(BuildInstallI2CodeCommand(name), "install i2code CLI")
}

func RemoveRepoDir(name string) error {
	return runCommand([]string{"limactl", "shell", name, "--", "rm", "-rf", "repo"}, "remove repo directory")
}

func runCommand(args []string, description string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to %s: %w", description, err)
	}
	return nil
}

func InstallUsingSDKMAN(name string) error {
	cmd := exec.Command("limactl", "shell", name, "--", "bash", "-s")
	cmd.Stdin = strings.NewReader(installUsingSDKMANScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Java/Gradle via SDKMAN: %w", err)
	}

	return nil
}
