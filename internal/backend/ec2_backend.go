package backend

import (
	"context"
	"fmt"
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// EnsureBucketFunc bootstraps the Terraform remote-state bucket for a region and
// returns its name.
type EnsureBucketFunc func(ctx context.Context, region string) (string, error)

type EC2Backend struct {
	MetadataDir      string
	LookupEnvFunc    func(string) (string, bool)
	EnsureBucketFunc EnsureBucketFunc
}

func notYetImplemented() error {
	return fmt.Errorf("not yet implemented for --type ec2")
}

func (b *EC2Backend) Create(opts CreateOptions) error {
	region, err := ec2.RequireRegion(b.lookupEnv())
	if err != nil {
		return fmt.Errorf("create %q: %w", opts.Name, err)
	}

	if _, err := b.EnsureBucketFunc(context.Background(), region); err != nil {
		return fmt.Errorf("create %q: %w", opts.Name, err)
	}

	return fmt.Errorf("create %q: %w", opts.Name, notYetImplemented())
}

func (b *EC2Backend) lookupEnv() func(string) (string, bool) {
	if b.LookupEnvFunc != nil {
		return b.LookupEnvFunc
	}
	return os.LookupEnv
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
