package docker

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed Dockerfile
var dockerfileContent string

func BuildCheckDockerCommand() []string {
	return []string{"docker", "info"}
}

type WorktreeConfig struct {
	WorktreeHostPath string
	MainRepoHostPath string
	MainRepoDir      string
}

func BuildImageCommand(tag string, contextDir string, wt *WorktreeConfig) []string {
	args := []string{"docker", "build", "-t", tag}
	if wt != nil {
		args = append(args, "--build-arg", "WORKTREE_HOST_PATH="+wt.WorktreeHostPath)
		args = append(args, "--build-arg", "MAIN_REPO_HOST_PATH="+wt.MainRepoHostPath)
	}
	args = append(args, contextDir)
	return args
}

func BuildRunCommand(name, workDir, imageTag string, wt *WorktreeConfig) []string {
	args := []string{
		"docker", "run", "-d",
		"--name", name,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", fmt.Sprintf("%s:/home/isolarium/repo", workDir),
	}
	if wt != nil {
		args = append(args, "-v", fmt.Sprintf("%s:/home/isolarium/main-repo", wt.MainRepoDir))
	}
	args = append(args, imageTag)
	return args
}

func WriteDockerTempfile() (string, error) {
	dir, err := os.MkdirTemp("", "isolarium-docker-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfileContent), 0644); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return dir, nil
}
