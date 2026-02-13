package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cer/isolarium/internal/command"
	"github.com/cer/isolarium/internal/docker"
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
	default:
		return nil, fmt.Errorf("unknown environment type: %q", envType)
	}
}

func newDockerBackend() *DockerBackend {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return &DockerBackend{
		Runner:         command.ExecRunner{},
		MetadataDir:    filepath.Join(home, ".isolarium"),
		ImageTag:       "isolarium:latest",
		ContextDirFunc: docker.WriteDockerTempfile,
	}
}
