package backend

import "fmt"

// ResolveBackend returns the appropriate Backend implementation for the given
// environment type. Supported types are "vm" (LimaBackend) and "container"
// (DockerBackend).
func ResolveBackend(envType string) (Backend, error) {
	switch envType {
	case "vm":
		return &LimaBackend{}, nil
	case "container":
		return &DockerBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown environment type: %q", envType)
	}
}
