package ec2

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

// WipeDeps is what tearing down the shared infrastructure needs from outside
// this package: how to run terraform, which account and bucket to speak to, how
// to recover the host's SSH material and address, and where to report progress.
type WipeDeps struct {
	Runner             command.Runner
	ResolveAccountFunc func() (region, bucket string, err error)
	EnsureKeypairFunc  func(base string) (string, error)
	DetectPublicIPFunc func() (string, error)
	Out                io.Writer
	// ErrWriter carries terraform's own progress, which is diagnostic rather
	// than part of the wipe's report.
	ErrWriter io.Writer
}

// Wipe destroys the VPC, security group, and key pair every EC2 environment
// shares. It refuses while any environment still exists, because destroy is the
// per-environment verb and a wipe that silently terminated running agent
// sessions would be the wrong default.
func Wipe(base string, deps WipeDeps) error {
	names, err := ListInstanceNames(base)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return existingEnvironmentsError(names)
	}

	w := sharedInfrastructure{base: base, deps: deps}
	bucket, err := w.destroy()
	if err != nil {
		return err
	}
	if err := w.removeHostFiles(); err != nil {
		return err
	}
	w.reportRetainedBucket(bucket)
	return nil
}

func existingEnvironmentsError(names []string) error {
	lines := []string{
		fmt.Sprintf("refusing to wipe: %s still exist: %s",
			countedEnvironments(len(names)), strings.Join(names, ", ")),
	}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("  run isolarium destroy --type ec2 --name %s", name))
	}
	return fmt.Errorf("%s", strings.Join(append(lines, "then run isolarium ec2 wipe again"), "\n"))
}

func countedEnvironments(count int) string {
	if count == 1 {
		return "1 EC2 environment"
	}
	return fmt.Sprintf("%d EC2 environments", count)
}

// sharedInfrastructure is one wipe in progress, so each step reads against the
// infrastructure being torn down rather than passing the base and its
// dependencies around.
type sharedInfrastructure struct {
	base string
	deps WipeDeps
}

// destroy tears down everything the Terraform working directory still describes
// — which, with every instance file already gone, is exactly the shared VPC,
// security group, and key pair — and returns the bucket that outlives it.
func (s sharedInfrastructure) destroy() (bucket string, err error) {
	region, bucket, err := s.deps.ResolveAccountFunc()
	if err != nil {
		return "", err
	}

	publicKey, err := s.deps.EnsureKeypairFunc(s.base)
	if err != nil {
		return "", err
	}

	cidr, err := s.ingressCIDR()
	if err != nil {
		return "", err
	}

	terraform := NewTerraformRunner(s.deps.Runner, TerraformConfig{Base: s.base, Bucket: bucket, Region: region}, s.errOut())
	if err := terraform.Init(); err != nil {
		return "", err
	}
	return bucket, terraform.Destroy(map[string]string{
		"ingress_cidr": cidr,
		"public_key":   publicKey,
		"region":       region,
	})
}

// ingressCIDR warns and falls back to the last successfully detected address
// when detection fails, so that being off the network isolarium was created
// from never blocks tearing the infrastructure down.
func (s sharedInfrastructure) ingressCIDR() (string, error) {
	cidr, err := s.deps.DetectPublicIPFunc()
	if err == nil {
		return cidr, PersistIngressCIDR(s.base, cidr)
	}

	persisted, persistErr := ReadPersistedIngressCIDR(s.base)
	if persistErr != nil {
		return "", fmt.Errorf("%w; and no ingress CIDR was persisted: %v", err, persistErr)
	}
	s.print(fmt.Sprintf("warning: public IP detection failed (%v); falling back to the last known ingress CIDR %s", err, persisted))
	return persisted, nil
}

func (s sharedInfrastructure) removeHostFiles() error {
	if err := os.RemoveAll(TerraformDir(s.base)); err != nil {
		return fmt.Errorf("removing %s: %w", TerraformDir(s.base), err)
	}
	for _, path := range []string{PrivateKeyPath(s.base), PublicKeyPath(s.base), KnownHostsPath(s.base)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}
	return nil
}

// reportRetainedBucket names the bucket that survives the wipe: it costs
// approximately nothing, is harmless to reuse, and deleting a versioned bucket
// means enumerating every object version by hand.
func (s sharedInfrastructure) reportRetainedBucket(bucket string) {
	s.print(fmt.Sprintf("S3 state bucket %s was intentionally retained; see the README for manual removal", bucket))
}

func (s sharedInfrastructure) print(message string) {
	_, _ = fmt.Fprintln(s.out(), message)
}

func (s sharedInfrastructure) out() io.Writer {
	if s.deps.Out == nil {
		return os.Stdout
	}
	return s.deps.Out
}

func (s sharedInfrastructure) errOut() io.Writer {
	if s.deps.ErrWriter == nil {
		return os.Stderr
	}
	return s.deps.ErrWriter
}
