package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cer/isolarium/internal/command"
	"github.com/cer/isolarium/internal/nono"
)

type UnsupportedOperationError struct {
	Operation string
}

func (e *UnsupportedOperationError) Error() string {
	return fmt.Sprintf("%s is not yet supported for nono backend", e.Operation)
}

type NonoBackend struct {
	Runner      command.Runner
	MetadataDir string
}

func (b *NonoBackend) Create(name string, opts CreateOptions) error {
	creator := &nono.Creator{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
	}
	return creator.Create(name, opts.WorkDirectory)
}

func (b *NonoBackend) Destroy(name string) error {
	return &UnsupportedOperationError{Operation: "destroy"}
}

func (b *NonoBackend) Exec(name string, envVars map[string]string, args []string) (int, error) {
	return 1, &UnsupportedOperationError{Operation: "exec"}
}

func (b *NonoBackend) ExecInteractive(name string, envVars map[string]string, args []string) (int, error) {
	return 1, &UnsupportedOperationError{Operation: "exec-interactive"}
}

func (b *NonoBackend) OpenShell(name string, envVars map[string]string) (int, error) {
	return 1, &UnsupportedOperationError{Operation: "open-shell"}
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
