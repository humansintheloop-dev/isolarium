package backend

import "fmt"

type EC2Backend struct {
	MetadataDir string
}

func notYetImplemented() error {
	return fmt.Errorf("not yet implemented for --type ec2")
}

func (b *EC2Backend) Create(opts CreateOptions) error {
	return fmt.Errorf("create %q: %w", opts.Name, notYetImplemented())
}

func (b *EC2Backend) Destroy(name string) error {
	return notYetImplemented()
}

func (b *EC2Backend) Exec(req ExecRequest) (int, error) {
	return 1, notYetImplemented()
}

func (b *EC2Backend) ExecInteractive(req ExecRequest) (int, error) {
	return 1, notYetImplemented()
}

func (b *EC2Backend) OpenShell(req ExecRequest) (int, error) {
	return 1, notYetImplemented()
}

func (b *EC2Backend) GetState(name string) string {
	return "none"
}

func (b *EC2Backend) CopyCredentials(name string, credentials string) error {
	return notYetImplemented()
}
