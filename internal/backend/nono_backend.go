package backend

import (
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/nono"
)

type nonoExecFunc func(name string, envVars map[string]string, args []string, extraReadPaths []string) (int, error)

type NonoBackend struct {
	Runner              command.Runner
	MetadataDir         string
	ExecFunc            nonoExecFunc
	ExecInteractiveFunc nonoExecFunc
	OpenShellFunc       ShellFunc
	ExtraReadPaths      []string
}

func (b *NonoBackend) Create(opts CreateOptions) error {
	creator := &nono.Creator{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
	}
	return creator.Create(opts.Name, opts.WorkDirectory)
}

func (b *NonoBackend) Destroy(name string) error {
	destroyer := &nono.Destroyer{
		MetadataDir: b.MetadataDir,
	}
	return destroyer.Destroy(name)
}

func (b *NonoBackend) Exec(req ExecRequest) (int, error) {
	return b.ExecFunc(req.ContainerName, req.EnvVars, req.Args, b.ExtraReadPaths)
}

func (b *NonoBackend) ExecInteractive(req ExecRequest) (int, error) {
	return b.ExecInteractiveFunc(req.ContainerName, req.EnvVars, req.Args, b.ExtraReadPaths)
}

func (b *NonoBackend) OpenShell(req ExecRequest) (int, error) {
	return b.OpenShellFunc(req)
}

func (b *NonoBackend) GetState(name string) string {
	nonoDir := filepath.Join(b.MetadataDir, name, "nono")
	if info, err := os.Stat(nonoDir); err == nil && info.IsDir() {
		return "configured"
	}
	return "none"
}

func (b *NonoBackend) CopyCredentials(name string, credentials string) error {
	return nil
}
