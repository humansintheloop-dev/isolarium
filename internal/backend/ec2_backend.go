package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// EnsureBucketFunc bootstraps the Terraform remote-state bucket for a region and
// returns its name.
type EnsureBucketFunc func(ctx context.Context, region string) (string, error)

// ec2ExecFunc runs a command on the instance reachable at publicDNS, using the
// SSH material under base, and returns the remote command's exit code.
type ec2ExecFunc func(base, publicDNS string, cmd ec2.RemoteCommand) (int, error)

type EC2Backend struct {
	MetadataDir            string
	Runner                 command.Runner
	NowFunc                func() time.Time
	LookupEnvFunc          func(string) (string, bool)
	EnsureBucketFunc       EnsureBucketFunc
	ExtractScaffoldingFunc func(base string) error
	EnsureKeypairFunc      func(base string) (string, error)
	DetectPublicIPFunc     func() (string, error)
	ExecFunc               ec2ExecFunc
	ExecInteractiveFunc    ec2ExecFunc
}

// hostState is what the host contributes to a terraform apply.
type hostState struct {
	publicKey   string
	ingressCIDR string
}

// environmentPlan is everything an apply needs to know about the environment
// being launched and the AWS account it lands in.
type environmentPlan struct {
	name   string
	region string
	bucket string
	host   hostState
}

func (p environmentPlan) applyVariables() map[string]string {
	return map[string]string{
		"ingress_cidr": p.host.ingressCIDR,
		"public_key":   p.host.publicKey,
		"region":       p.region,
	}
}

func notYetImplemented() error {
	return fmt.Errorf("not yet implemented for --type ec2")
}

func (b *EC2Backend) Create(opts CreateOptions) error {
	if err := b.create(opts); err != nil {
		return fmt.Errorf("create %q: %w", opts.Name, err)
	}
	return nil
}

func (b *EC2Backend) create(opts CreateOptions) error {
	region, err := ec2.RequireRegion(b.lookupEnv())
	if err != nil {
		return err
	}

	bucket, err := b.EnsureBucketFunc(context.Background(), region)
	if err != nil {
		return err
	}

	host, err := b.provisionHostState()
	if err != nil {
		return err
	}

	return b.launchInstance(environmentPlan{name: opts.Name, region: region, bucket: bucket, host: host})
}

// provisionHostState extracts the Terraform scaffolding, ensures the Ed25519
// keypair, and pins SSH ingress to the host's current public address.
func (b *EC2Backend) provisionHostState() (hostState, error) {
	if err := b.ExtractScaffoldingFunc(b.MetadataDir); err != nil {
		return hostState{}, err
	}

	publicKey, err := b.EnsureKeypairFunc(b.MetadataDir)
	if err != nil {
		return hostState{}, err
	}

	cidr, err := b.DetectPublicIPFunc()
	if err != nil {
		return hostState{}, err
	}
	if err := ec2.PersistIngressCIDR(b.MetadataDir, cidr); err != nil {
		return hostState{}, err
	}
	return hostState{publicKey: publicKey, ingressCIDR: cidr}, nil
}

// launchInstance describes the environment as one generated Terraform file,
// applies it, and records where the resulting instance can be reached. The
// cloud-init document arrives in Steel Thread 2 through the userData parameter.
func (b *EC2Backend) launchInstance(plan environmentPlan) error {
	if err := ec2.WriteInstanceFile(b.MetadataDir, plan.name, ""); err != nil {
		return err
	}

	terraform := ec2.NewTerraformRunner(b.Runner, b.MetadataDir, plan.bucket, plan.region)
	if err := terraform.Init(); err != nil {
		return err
	}
	if err := terraform.Apply(plan.applyVariables()); err != nil {
		return err
	}

	output, err := terraform.OutputJSON()
	if err != nil {
		return err
	}
	instanceID, publicDNS, err := ec2.ParseTerraformOutput(output, plan.name)
	if err != nil {
		return err
	}

	return ec2.NewMetadataStore(b.MetadataDir, plan.name).Write(ec2.Metadata{
		InstanceID: instanceID,
		PublicDNS:  publicDNS,
		Region:     plan.region,
		CreatedAt:  b.now(),
	})
}

func (b *EC2Backend) lookupEnv() func(string) (string, bool) {
	if b.LookupEnvFunc != nil {
		return b.LookupEnvFunc
	}
	return os.LookupEnv
}

func (b *EC2Backend) now() time.Time {
	if b.NowFunc != nil {
		return b.NowFunc()
	}
	return time.Now().UTC()
}

func (b *EC2Backend) Destroy(name string) error {
	return notYetImplemented()
}

func (b *EC2Backend) Exec(req ExecRequest) (int, error) {
	return b.runOnInstance(b.ExecFunc, req)
}

func (b *EC2Backend) ExecInteractive(req ExecRequest) (int, error) {
	return b.runOnInstance(b.ExecInteractiveFunc, req)
}

// runOnInstance resolves where the environment can be reached from the metadata
// recorded at create time, so running a command costs no AWS or terraform call.
// The remote working directory arrives with the repository clone.
func (b *EC2Backend) runOnInstance(run ec2ExecFunc, req ExecRequest) (int, error) {
	meta, err := ec2.NewMetadataStore(b.MetadataDir, req.ContainerName).Read()
	if err != nil {
		return 1, err
	}
	return run(b.MetadataDir, meta.PublicDNS, ec2.RemoteCommand{EnvVars: req.EnvVars, Args: req.Args})
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
