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

func BuildImageCommand(tag string, contextDir string) []string {
	return []string{"docker", "build", "-t", tag, contextDir}
}

func BuildRunCommand(name, workDir, imageTag string) []string {
	return []string{
		"docker", "run", "-d",
		"--name", name,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", fmt.Sprintf("%s:/home/isolarium/repo", workDir),
		imageTag,
	}
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
