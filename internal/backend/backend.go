package backend

type CreateOptions struct {
	Name          string
	WorkDirectory string
}

type ExecRequest struct {
	ContainerName string
	EnvVars       map[string]string
	Args          []string
}

type Backend interface {
	Create(opts CreateOptions) error
	Destroy(name string) error
	Exec(req ExecRequest) (int, error)
	ExecInteractive(req ExecRequest) (int, error)
	OpenShell(req ExecRequest) (int, error)
	GetState(name string) string
	CopyCredentials(name string, credentials string) error
}
