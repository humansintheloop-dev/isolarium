package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrNoEnvironmentFound = errors.New("no environment found")

var knownEnvironmentTypes = []string{"vm", "container"}

func ResolveEnvironmentType(baseDir, name string) (string, error) {
	envDir := filepath.Join(baseDir, name)

	var found []string
	for _, envType := range knownEnvironmentTypes {
		typeDir := filepath.Join(envDir, envType)
		if info, err := os.Stat(typeDir); err == nil && info.IsDir() {
			found = append(found, envType)
		}
	}

	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("%w for %q", ErrNoEnvironmentFound, name)
	default:
		return "", fmt.Errorf("multiple environments found for %q: specify --type to disambiguate", name)
	}
}
