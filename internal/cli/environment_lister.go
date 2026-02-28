package cli

import (
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/status"
)

type defaultEnvironmentLister struct {
	baseDir  string
	resolver BackendResolver
}

func newDefaultEnvironmentLister(resolver BackendResolver) *defaultEnvironmentLister {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return &defaultEnvironmentLister{
		baseDir:  filepath.Join(home, ".isolarium"),
		resolver: resolver,
	}
}

func (l *defaultEnvironmentLister) List(nameFilter, typeFilter string) []status.EnvironmentStatus {
	stateProvider := func(name, envType string) string {
		b, err := l.resolver(envType)
		if err != nil {
			return "unknown"
		}
		return b.GetState(name)
	}

	var opts []status.ListOption
	if nameFilter != "" {
		opts = append(opts, status.WithName(nameFilter))
	}
	if typeFilter != "" {
		opts = append(opts, status.WithType(typeFilter))
	}

	return status.ListAllEnvironments(l.baseDir, stateProvider, opts...)
}

var _ EnvironmentLister = (*defaultEnvironmentLister)(nil)
