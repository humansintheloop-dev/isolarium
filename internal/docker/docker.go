package docker

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

//go:embed Dockerfile
var dockerfileContent string

func ImageTagForContainer(containerName string) string {
	return "isolarium-" + containerName + ":latest"
}

const i2codeRepo = "https://github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git"

func BuildI2CodeHeadSHACommand() []string {
	return []string{"git", "ls-remote", i2codeRepo, "HEAD"}
}

func BuildCheckDockerCommand() []string {
	return []string{"docker", "info"}
}

type WorktreeConfig struct {
	WorktreeHostPath string
	MainRepoHostPath string
	MainRepoDir      string
}

func BuildImageCommand(tag string, contextDir string, wt *WorktreeConfig, buildArgs map[string]string) []string {
	args := []string{"docker", "build", "-t", tag}
	args = append(args, "--build-arg", "HOST_UID="+strconv.Itoa(os.Getuid()))
	if wt != nil {
		args = append(args, "--build-arg", "WORKTREE_HOST_PATH="+wt.WorktreeHostPath)
		args = append(args, "--build-arg", "MAIN_REPO_HOST_PATH="+wt.MainRepoHostPath)
	}
	sortedKeys := make([]string, 0, len(buildArgs))
	for k := range buildArgs {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	for _, k := range sortedKeys {
		args = append(args, "--build-arg", k+"="+buildArgs[k])
	}
	args = append(args, contextDir)
	return args
}

func BuildRunCommand(name, workDir, imageTag string, wt *WorktreeConfig) []string {
	homeDir, _ := os.UserHomeDir()
	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")

	args := []string{
		"docker", "run", "-d",
		"--name", name,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", fmt.Sprintf("%s:/home/isolarium/repo", workDir),
		"-v", fmt.Sprintf("%s:/home/isolarium/.ssh/known_hosts:ro", knownHostsPath),
	}
	if wt != nil {
		args = append(args, "-v", fmt.Sprintf("%s:/home/isolarium/main-repo", wt.MainRepoDir))
	}
	args = append(args, imageTag)
	return args
}

func BuildContainerImageIDCommand(containerName string) []string {
	return []string{"docker", "inspect", "--format", "{{.Image}}", containerName}
}

func BuildImageIDCommand(imageTag string) []string {
	return []string{"docker", "inspect", "--format", "{{.Id}}", imageTag}
}

func WriteDockerTempfile() (string, error) {
	dir, err := os.MkdirTemp("", "isolarium-docker-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfileContent), 0644); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return dir, nil
}
