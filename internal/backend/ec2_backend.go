package backend

import (
	"context"
	"fmt"
	"io"
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
	Out                    io.Writer
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
	region, bucket, err := b.resolveAWSAccount()
	if err != nil {
		return err
	}

	host, err := b.provisionHostState()
	if err != nil {
		return err
	}

	return b.launchInstance(environmentPlan{name: opts.Name, region: region, bucket: bucket, host: host})
}

// resolveAWSAccount yields the region and the remote-state bucket every
// terraform invocation needs, whether an environment is being launched or torn
// down. Bootstrapping the bucket is idempotent, so teardown can share it.
func (b *EC2Backend) resolveAWSAccount() (region, bucket string, err error) {
	region, err = ec2.RequireRegion(b.lookupEnv())
	if err != nil {
		return "", "", err
	}

	bucket, err = b.EnsureBucketFunc(context.Background(), region)
	if err != nil {
		return "", "", err
	}
	return region, bucket, nil
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

func (b *EC2Backend) out() io.Writer {
	if b.Out != nil {
		return b.Out
	}
	return os.Stdout
}

func (b *EC2Backend) Destroy(name string) error {
	teardown := ec2Teardown{backend: b, name: name}
	if !teardown.environmentExists() {
		b.print("no EC2 environment to destroy")
		return nil
	}
	if err := teardown.run(); err != nil {
		return fmt.Errorf("destroy %q: %w", name, err)
	}
	return nil
}

// ec2Teardown is one environment's destruction in progress, so that each of its
// steps reads against the environment rather than passing its name around.
type ec2Teardown struct {
	backend *EC2Backend
	name    string
}

func (t ec2Teardown) environmentExists() bool {
	return ec2.InstanceFileExists(t.backend.MetadataDir, t.name)
}

// run removes the environment's Terraform file and re-applies, rather than
// targeting the instance for destruction. A resource left in state with no
// configuration is always planned for destruction, so an interrupted destroy
// converges when it is re-run.
func (t ec2Teardown) run() error {
	plan, err := t.plan()
	if err != nil {
		return err
	}

	publicDNS := t.recordedPublicDNS()
	base := t.backend.MetadataDir
	if err := ec2.RemoveInstanceFile(base, t.name); err != nil {
		return err
	}

	terraform := ec2.NewTerraformRunner(t.backend.Runner, base, plan.bucket, plan.region)
	if err := terraform.Init(); err != nil {
		return err
	}
	if err := terraform.Apply(plan.applyVariables()); err != nil {
		return err
	}

	if publicDNS != "" {
		if err := ec2.EvictKnownHost(base, publicDNS, t.backend.Runner); err != nil {
			return err
		}
	}
	return ec2.NewMetadataStore(base, t.name).Cleanup()
}

// plan gathers everything the teardown apply needs before anything on disk is
// touched, so a failure to resolve it leaves the environment intact.
func (t ec2Teardown) plan() (environmentPlan, error) {
	region, bucket, err := t.backend.resolveAWSAccount()
	if err != nil {
		return environmentPlan{}, err
	}

	publicKey, err := t.backend.EnsureKeypairFunc(t.backend.MetadataDir)
	if err != nil {
		return environmentPlan{}, err
	}

	cidr, err := t.ingressCIDR()
	if err != nil {
		return environmentPlan{}, err
	}

	return environmentPlan{
		name:   t.name,
		region: region,
		bucket: bucket,
		host:   hostState{publicKey: publicKey, ingressCIDR: cidr},
	}, nil
}

// ingressCIDR warns and falls back to the last successfully detected address
// when detection fails, so that being off the network isolarium was created
// from never blocks tearing an instance down.
func (t ec2Teardown) ingressCIDR() (string, error) {
	base := t.backend.MetadataDir

	cidr, err := t.backend.DetectPublicIPFunc()
	if err == nil {
		return cidr, ec2.PersistIngressCIDR(base, cidr)
	}

	persisted, persistErr := ec2.ReadPersistedIngressCIDR(base)
	if persistErr != nil {
		return "", fmt.Errorf("%w; and no ingress CIDR was persisted: %v", err, persistErr)
	}
	t.backend.print(fmt.Sprintf("warning: public IP detection failed (%v); falling back to the last known ingress CIDR %s", err, persisted))
	return persisted, nil
}

// recordedPublicDNS is empty when create was interrupted before it recorded any
// metadata, which leaves nothing to evict from known_hosts but still leaves an
// instance to terminate.
func (t ec2Teardown) recordedPublicDNS() string {
	meta, err := ec2.NewMetadataStore(t.backend.MetadataDir, t.name).Read()
	if err != nil {
		return ""
	}
	return meta.PublicDNS
}

func (b *EC2Backend) print(message string) {
	_, _ = fmt.Fprintln(b.out(), message)
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
