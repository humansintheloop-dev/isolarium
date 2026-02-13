package backend

// CreateOptions holds options for creating an environment.
type CreateOptions struct {
	WorkDirectory string
}

// Backend defines the interface for isolation backends (VM or container).
type Backend interface {
	Create(name string, opts CreateOptions) error
	Destroy(name string) error
	Exec(name string, envVars map[string]string, args []string) (int, error)
	ExecInteractive(name string, envVars map[string]string, args []string) (int, error)
	OpenShell(name string, envVars map[string]string) (int, error)
	GetState(name string) string
	CopyCredentials(name string, credentials string) error
}
