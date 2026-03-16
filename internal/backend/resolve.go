package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/docker"
	"github.com/humansintheloop-dev/isolarium/internal/git"
	"github.com/humansintheloop-dev/isolarium/internal/nono"
)

// ResolveBackend returns the appropriate Backend implementation for the given
// environment type. Supported types are "vm" (LimaBackend) and "container"
// (DockerBackend).
func ResolveBackend(envType string) (Backend, error) {
	switch envType {
	case "vm":
		return &LimaBackend{}, nil
	case "container":
		return newDockerBackend(), nil
	case "nono":
		return newNonoBackend(), nil
	default:
		return nil, fmt.Errorf("unknown environment type: %q", envType)
	}
}

func newNonoBackend() *NonoBackend {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return &NonoBackend{
		Runner:              command.ExecRunner{},
		MetadataDir:         filepath.Join(home, ".isolarium"),
		ExecFunc:            nono.ExecCommand,
		ExecInteractiveFunc: nono.ExecInteractiveCommand,
		OpenShellFunc:       func(req ExecRequest) (int, error) { return nono.OpenShell(req.ContainerName, req.EnvVars) },
	}
}

func newDockerBackend() *DockerBackend {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return &DockerBackend{
		Runner:              command.ExecRunner{},
		MetadataDir:         filepath.Join(home, ".isolarium"),
		ImageTag:            "",
		ContextDirFunc:      docker.WriteDockerTempfile,
		ExecFunc:            func(req ExecRequest) (int, error) { return docker.ExecCommand(req.ContainerName, req.EnvVars, req.Args) },
		ExecInteractiveFunc: func(req ExecRequest) (int, error) { return docker.ExecInteractiveCommand(req.ContainerName, req.EnvVars, req.Args) },
		OpenShellFunc:       func(req ExecRequest) (int, error) { return docker.OpenShell(req.ContainerName, req.EnvVars) },
		CopyCredentialsFunc: docker.CopyClaudeCredentials,
		DetectWorktreeFunc:  git.DetectWorktree,
	}
}
