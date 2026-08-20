package backend

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/config"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
	"github.com/humansintheloop-dev/isolarium/internal/envscript"
	"github.com/humansintheloop-dev/isolarium/internal/hostscript"
)

// ec2IsolationType is the name this backend answers to, and the value its
// pid.yaml scripts see in ISOLARIUM_TYPE.
const ec2IsolationType = "ec2"

// EnsureBucketFunc bootstraps the Terraform remote-state bucket for a region and
// returns its name.
type EnsureBucketFunc func(ctx context.Context, region string) (string, error)

// ec2ExecFunc runs a command on the instance reachable at publicDNS, using the
// SSH material under base, and returns the remote command's exit code.
type ec2ExecFunc = ec2.RemoteRunner

// SessionNameFunc chooses which tmux session an interactive invocation joins on
// the instance reachable at publicDNS, resolved per invocation because the
// answer depends on what is already running there.
type SessionNameFunc func(base, publicDNS string) (string, error)

// ec2CopyCredentialsFunc places the host's Claude credentials on the instance
// reachable at publicDNS, using the SSH material under base.
type ec2CopyCredentialsFunc func(base, publicDNS, credentials string) error

// DescribeInstanceFunc asks AWS where an instance can currently be reached and
// what state it is in, given the region and instance ID create recorded.
type DescribeInstanceFunc func(ctx context.Context, region, instanceID string) (publicDNS, state string, err error)

type EC2Backend struct {
	MetadataDir            string
	Runner                 command.Runner
	NowFunc                func() time.Time
	LookupEnvFunc          func(string) (string, bool)
	EnsureBucketFunc       EnsureBucketFunc
	ExtractScaffoldingFunc func(base string) error
	EnsureKeypairFunc      func(base string) (string, error)
	DetectPublicIPFunc     func() (string, error)
	SleepFunc              func(time.Duration)
	ExecFunc               ec2ExecFunc
	ExecInteractiveFunc    ec2ExecFunc
	CaptureFunc            ec2.RemoteOutputRunner
	SessionNameFunc        SessionNameFunc
	CopyCredentialsFunc    ec2CopyCredentialsFunc
	DescribeInstanceFunc   DescribeInstanceFunc
	Out                    io.Writer
	ErrWriter              io.Writer
}

// UseNewSession makes every interactive invocation join a session no other one
// holds. It is what --new-session asks for: an additional session, never a
// replaced one.
func (b *EC2Backend) UseNewSession() {
	b.SessionNameFunc = func(base, publicDNS string) (string, error) {
		return ec2.NewInstanceQuery(base, publicDNS, b.capture()).ResolveNewSessionName()
	}
}

func (b *EC2Backend) capture() ec2.RemoteOutputRunner {
	if b.CaptureFunc != nil {
		return b.CaptureFunc
	}
	return ec2.CaptureCommand
}

// sessionName defaults to the single shared session, so an ordinary run finds
// the agent an earlier one left working.
func (b *EC2Backend) sessionName(publicDNS string) (string, error) {
	if b.SessionNameFunc == nil {
		return ec2.DefaultSessionName, nil
	}
	return b.SessionNameFunc(b.MetadataDir, publicDNS)
}

// launchedInstance is where terraform says a newly applied instance can be
// reached.
type launchedInstance struct {
	id        string
	publicDNS string
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

func (b *EC2Backend) Create(opts CreateOptions) error {
	if err := b.create(opts); err != nil {
		return fmt.Errorf("create %q: %w", opts.Name, err)
	}
	return nil
}

func (b *EC2Backend) create(opts CreateOptions) error {
	// The project's own configuration is read before the account is touched, so
	// a pid.yaml that names an escaping script path costs nothing to reject.
	cfg, err := config.LoadPidConfig(opts.WorkDirectory)
	if err != nil {
		return fmt.Errorf("loading pid.yaml: %w", err)
	}

	region, bucket, err := b.resolveAWSAccount()
	if err != nil {
		return err
	}

	host, err := b.provisionHostState()
	if err != nil {
		return err
	}

	plan := environmentPlan{name: opts.Name, region: region, bucket: bucket, host: host}
	instance, err := b.launchInstance(plan, opts.Repository)
	if err != nil {
		return err
	}
	return b.runConfiguredScripts(cfg, opts, instance)
}

// runConfiguredScripts carries out the pid.yaml ec2 hooks now that the instance
// holds the repository: the creation scripts and the post-creation env scripts
// run inside it, while the host scripts run on the host.
func (b *EC2Backend) runConfiguredScripts(cfg *config.PidConfig, opts CreateOptions, instance launchedInstance) error {
	if cfg == nil {
		return nil
	}

	create := cfg.EC2.Create
	onInstance := b.instanceScriptRunner(instance)
	if err := envscript.RunCreationScripts(create.CreationScripts, opts.Name, ec2IsolationType, onInstance); err != nil {
		return err
	}
	if err := hostscript.RunHostScripts(create.PostCreationScripts.HostScripts, opts.WorkDirectory, opts.Name, ec2IsolationType); err != nil {
		return err
	}
	return envscript.RunEnvScripts(create.PostCreationScripts.EnvScripts, opts.Name, ec2IsolationType, onInstance)
}

// instanceScriptRunner runs a pid.yaml script from the repository on the
// instance, which is where its path resolves, and reports a rejected script as
// an error — a provisioning step has no exit status worth propagating.
func (b *EC2Backend) instanceScriptRunner(instance launchedInstance) envscript.EnvExecFunc {
	return func(envVars map[string]string, args []string) (int, error) {
		exitCode, err := b.ExecFunc(b.MetadataDir, instance.publicDNS, ec2.RemoteCommand{
			Workdir: ec2.RemoteRepoDir,
			EnvVars: envVars,
			Args:    args,
		})
		if err != nil {
			return exitCode, err
		}
		if exitCode != 0 {
			return exitCode, fmt.Errorf("the instance rejected it with exit code %d", exitCode)
		}
		return exitCode, nil
	}
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

// launchInstance takes an environment from a Terraform description to an
// instance that is ready to work in: applied, reachable, provisioned, and
// holding the repository.
func (b *EC2Backend) launchInstance(plan environmentPlan, repository RepositorySource) (launchedInstance, error) {
	b.print("Creating EC2 instance...")
	instance, err := b.applyInstance(plan)
	if err != nil {
		return launchedInstance{}, err
	}

	source, err := repository()
	if err != nil {
		return launchedInstance{}, err
	}

	// Recording where the instance can be reached before waiting on it keeps a
	// timed-out provisioning destroyable.
	if err := b.recordMetadata(plan, instance, source); err != nil {
		return launchedInstance{}, err
	}
	if err := b.provisionInstance(instance.publicDNS, source); err != nil {
		return launchedInstance{}, err
	}
	return instance, nil
}

// applyInstance describes the environment as one generated Terraform file and
// applies it. The cloud-init document rides along as user_data, so the instance
// provisions its toolchain on first boot.
func (b *EC2Backend) applyInstance(plan environmentPlan) (launchedInstance, error) {
	if err := ec2.WriteInstanceFile(b.MetadataDir, plan.name, ec2.RenderUserData()); err != nil {
		return launchedInstance{}, err
	}

	terraform := ec2.NewTerraformRunner(b.Runner, b.MetadataDir, plan.bucket, plan.region)
	if err := terraform.Init(); err != nil {
		return launchedInstance{}, err
	}
	if err := terraform.Apply(plan.applyVariables()); err != nil {
		return launchedInstance{}, err
	}

	output, err := terraform.OutputJSON()
	if err != nil {
		return launchedInstance{}, err
	}
	instanceID, publicDNS, err := ec2.ParseTerraformOutput(output, plan.name)
	if err != nil {
		return launchedInstance{}, err
	}
	return launchedInstance{id: instanceID, publicDNS: publicDNS}, nil
}

func (b *EC2Backend) recordMetadata(plan environmentPlan, instance launchedInstance, source ec2.RepositorySpec) error {
	return ec2.NewMetadataStore(b.MetadataDir, plan.name).Write(ec2.Metadata{
		InstanceID: instance.id,
		PublicDNS:  instance.publicDNS,
		Region:     plan.region,
		Owner:      source.Owner,
		Repo:       source.Repo,
		Branch:     source.Branch,
		CreatedAt:  b.now(),
	})
}

// provisionInstance waits out the gap between an instance AWS calls running and
// one that can actually be worked in, then places the repository inside it.
func (b *EC2Backend) provisionInstance(publicDNS string, source ec2.RepositorySpec) error {
	// The readiness probes are queries rather than plain commands: cloud-init's
	// own report of a degraded run is what names the modules that failed.
	query := ec2.NewInstanceQuery(b.MetadataDir, publicDNS, b.capture())

	if err := ec2.WaitForSSH(query, b.sleep()); err != nil {
		return err
	}

	b.print("Waiting for cloud-init...")
	if err := ec2.WaitForCloudInit(query, b.sleep()); err != nil {
		return err
	}

	b.print("Cloning repository...")
	return ec2.PlaceRepository(ec2.NewInstanceSession(b.MetadataDir, publicDNS, b.ExecFunc), source)
}

func (b *EC2Backend) sleep() ec2.SleepFunc {
	if b.SleepFunc != nil {
		return b.SleepFunc
	}
	return time.Sleep
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

// errOut carries notices that are about the connection rather than about the
// command, so they stay out of the command's own output.
func (b *EC2Backend) errOut() io.Writer {
	if b.ErrWriter != nil {
		return b.ErrWriter
	}
	return os.Stderr
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

// ExecInteractive runs the command inside the instance's tmux session, so a
// dropped connection leaves it running rather than killing it.
func (b *EC2Backend) ExecInteractive(req ExecRequest) (int, error) {
	session, err := b.attachSession(req.ContainerName)
	if err != nil {
		return 1, err
	}
	return b.ExecInteractiveFunc(b.MetadataDir, session.publicDNS, ec2.RemoteCommand{
		Workdir: ec2.RemoteRepoDir,
		EnvVars: req.EnvVars,
		Args:    ec2.BuildTmuxCommand(session.name, req.Args),
	})
}

// runOnInstance resolves where the environment can be reached from the metadata
// recorded at create time, so running a command costs no AWS or terraform call.
// Commands run from the repository create placed on the instance.
func (b *EC2Backend) runOnInstance(run ec2ExecFunc, req ExecRequest) (int, error) {
	meta, err := ec2.NewMetadataStore(b.MetadataDir, req.ContainerName).Read()
	if err != nil {
		return 1, err
	}
	return run(b.MetadataDir, meta.PublicDNS, ec2.RemoteCommand{
		Workdir: ec2.RemoteRepoDir,
		EnvVars: req.EnvVars,
		Args:    req.Args,
	})
}

func (b *EC2Backend) OpenShell(req ExecRequest) (int, error) {
	session, err := b.attachSession(req.ContainerName)
	if err != nil {
		return 1, err
	}
	return ec2.OpenShell(b.instanceSession(session.publicDNS, b.ExecInteractiveFunc), session.name, req.EnvVars)
}

// persistentSession is the tmux session an interactive command joins on the
// instance, and where that instance can be reached.
type persistentSession struct {
	publicDNS string
	name      string
}

// attachSession resolves the session an interactive command is about to join,
// announcing a reattach first because tmux discards the command it was handed
// once the session already exists.
func (b *EC2Backend) attachSession(name string) (persistentSession, error) {
	meta, err := ec2.NewMetadataStore(b.MetadataDir, name).Read()
	if err != nil {
		return persistentSession{}, err
	}

	sessionName, err := b.sessionName(meta.PublicDNS)
	if err != nil {
		return persistentSession{}, err
	}

	session := persistentSession{publicDNS: meta.PublicDNS, name: sessionName}
	ec2.AnnounceReattach(b.errOut(), b.instanceSession(session.publicDNS, b.ExecFunc), session.name)
	return session, nil
}

func (b *EC2Backend) instanceSession(publicDNS string, run ec2ExecFunc) ec2.InstanceSession {
	return ec2.NewInstanceSession(b.MetadataDir, publicDNS, run)
}

// GetState reports an environment that was never created, or whose instance AWS
// has already reclaimed, as absent; a lookup it could not make at all is
// reported as unknown, so status degrades rather than fails when the AWS
// credentials are missing.
func (b *EC2Backend) GetState(name string) string {
	meta, err := ec2.NewMetadataStore(b.MetadataDir, name).Read()
	if err != nil {
		return "none"
	}

	_, awsState, err := b.describeInstance()(context.Background(), meta.Region, meta.InstanceID)
	if err != nil {
		return "unknown"
	}
	return isolariumState(awsState)
}

// isolariumState translates the AWS instance lifecycle into the vocabulary the
// status layer already speaks. An instance on its way out is reported as absent
// rather than as stopping, because nothing can be run in it again.
func isolariumState(awsState string) string {
	switch awsState {
	case "running":
		return "running"
	case "stopped", "stopping":
		return "stopped"
	case "pending":
		return "pending"
	case "shutting-down", "terminated":
		return "none"
	default:
		return "unknown"
	}
}

func (b *EC2Backend) describeInstance() DescribeInstanceFunc {
	if b.DescribeInstanceFunc != nil {
		return b.DescribeInstanceFunc
	}
	return ec2.LookupInstance
}

// CopyCredentials carries the host's Claude credentials to the instance, which
// only overwrites what is already there when the host's copy is the fresher one.
func (b *EC2Backend) CopyCredentials(name string, credentials string) error {
	meta, err := ec2.NewMetadataStore(b.MetadataDir, name).Read()
	if err != nil {
		return err
	}
	return b.copyCredentials()(b.MetadataDir, meta.PublicDNS, credentials)
}

func (b *EC2Backend) copyCredentials() ec2CopyCredentialsFunc {
	if b.CopyCredentialsFunc != nil {
		return b.CopyCredentialsFunc
	}
	return func(base, publicDNS, credentials string) error {
		return ec2.NewInstanceQuery(base, publicDNS, b.capture()).CopyClaudeCredentials(credentials)
	}
}
