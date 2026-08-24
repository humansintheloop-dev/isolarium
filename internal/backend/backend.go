package backend

import "github.com/humansintheloop-dev/isolarium/internal/ec2"

// RepositorySource resolves where a new environment's repository comes from. It
// is a function rather than a value because resolving it pushes the current
// branch and mints a short-lived clone token, which must not happen for a create
// that fails before an instance exists.
type RepositorySource func() (ec2.RepositorySpec, error)

type CreateOptions struct {
	Name          string
	WorkDirectory string
	Repository    RepositorySource
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
