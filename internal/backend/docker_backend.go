package backend

import "errors"

// ErrNotImplemented is returned by DockerBackend methods that are not yet implemented.
var ErrNotImplemented = errors.New("docker backend not implemented")

// DockerBackend implements the Backend interface using Docker containers.
type DockerBackend struct{}

func (b *DockerBackend) Create(name string, opts CreateOptions) error {
	return ErrNotImplemented
}

func (b *DockerBackend) Destroy(name string) error {
	return ErrNotImplemented
}

func (b *DockerBackend) Exec(name string, envVars map[string]string, args []string) (int, error) {
	return 0, ErrNotImplemented
}

func (b *DockerBackend) ExecInteractive(name string, envVars map[string]string, args []string) (int, error) {
	return 0, ErrNotImplemented
}

func (b *DockerBackend) GetState(name string) string {
	return "not implemented"
}

func (b *DockerBackend) CopyCredentials(name string, credentials string) error {
	return ErrNotImplemented
}
