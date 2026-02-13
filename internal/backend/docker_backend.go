package backend

import (
	"os"

	"github.com/cer/isolarium/internal/command"
	"github.com/cer/isolarium/internal/docker"
)

// ExecFunc is the function signature for executing commands in a container.
type ExecFunc func(name string, envVars map[string]string, args []string) (int, error)

// ShellFunc is the function signature for opening an interactive shell in a container.
type ShellFunc func(name string, envVars map[string]string) (int, error)

// CopyCredentialsFunc is the function signature for copying credentials into a container.
type CopyCredentialsFunc func(name, credentials string) error

// DockerBackend implements the Backend interface using Docker containers.
type DockerBackend struct {
	Runner              command.Runner
	MetadataDir         string
	ImageTag            string
	ContextDirFunc      func() (string, error)
	ExecFunc            ExecFunc
	ExecInteractiveFunc ExecFunc
	OpenShellFunc       ShellFunc
	CopyCredentialsFunc CopyCredentialsFunc
}

func (b *DockerBackend) Create(name string, opts CreateOptions) error {
	contextDir, err := b.ContextDirFunc()
	if err != nil {
		return err
	}
	defer os.RemoveAll(contextDir)

	creator := &docker.Creator{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
		ImageTag:    b.ImageTag,
	}
	return creator.Create(name, opts.WorkDirectory, contextDir)
}

func (b *DockerBackend) Destroy(name string) error {
	destroyer := &docker.Destroyer{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
	}
	return destroyer.Destroy(name)
}

func (b *DockerBackend) Exec(name string, envVars map[string]string, args []string) (int, error) {
	return b.ExecFunc(name, envVars, args)
}

func (b *DockerBackend) ExecInteractive(name string, envVars map[string]string, args []string) (int, error) {
	return b.ExecInteractiveFunc(name, envVars, args)
}

func (b *DockerBackend) OpenShell(name string, envVars map[string]string) (int, error) {
	return b.OpenShellFunc(name, envVars)
}

func (b *DockerBackend) GetState(name string) string {
	checker := &docker.StateChecker{Runner: b.Runner}
	return checker.GetState(name)
}

func (b *DockerBackend) CopyCredentials(name string, credentials string) error {
	return b.CopyCredentialsFunc(name, credentials)
}
