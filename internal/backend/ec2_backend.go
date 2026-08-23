package backend

import (
	"context"
	"errors"
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
	CheckIPFunc            ec2.HTTPGetFunc
	SleepFunc              func(time.Duration)
	ExecFunc               ec2ExecFunc
	ExecInteractiveFunc    ec2ExecFunc
	ExecInSessionFunc      ec2ExecFunc
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

// execInSession is the transport for a command that must run inside tmux while
// isolarium itself may have no terminal, as it does not under i2code.
func (b *EC2Backend) execInSession() ec2ExecFunc {
	if b.ExecInSessionFunc != nil {
		return b.ExecInSessionFunc
	}
	return ec2.ExecInSessionCommand
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

func (p environmentPlan) terraformConfig(base string) ec2.TerraformConfig {
	return ec2.TerraformConfig{Base: base, Bucket: p.bucket, Region: p.region}
}

func (p environmentPlan) applyVariables() map[string]string {
	return map[string]string{
		"ingress_cidr": p.host.ingressCIDR,
		"public_key":   p.host.publicKey,
		"region":       p.region,
	}
}

func (b *EC2Backend) Create(opts CreateOptions) error {
	// The name is reported unwrapped because it names the flag the caller typed,
	// and it is checked before anything else so a bad name never reaches AWS.
	if err := ec2.ValidateName(opts.Name); err != nil {
		return err
	}
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

	region, bucket, err := b.resolveAWSAccountForCreate()
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

// resolveAWSAccountForCreate additionally rejects a host terraform too old for
// the S3 backend's native state locking. Teardown skips the check because an
// existing environment was launched by a terraform that already passed it, and
// refusing to destroy it would strand a running instance.
func (b *EC2Backend) resolveAWSAccountForCreate() (region, bucket string, err error) {
	region, err = ec2.RequireRegion(b.lookupEnv())
	if err != nil {
		return "", "", err
	}

	// The gate comes before the bucket so an unsupported host creates nothing in
	// the account.
	if err = ec2.CheckTerraformVersion(b.Runner); err != nil {
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

	cidr, err := b.resolveIngressCIDR(ec2.OpCreate)
	if err != nil {
		return hostState{}, err
	}
	return hostState{publicKey: publicKey, ingressCIDR: cidr}, nil
}

// resolveIngressCIDR applies the per-operation detection-failure policy and
// reports any fallback on stderr, where a warning belongs rather than in the
// command's own output.
func (b *EC2Backend) resolveIngressCIDR(op ec2.Operation) (string, error) {
	cidr, warning, err := ec2.ResolveIngressCIDR(b.MetadataDir, op, b.CheckIPFunc)
	if err != nil {
		return "", err
	}
	if warning != "" {
		b.printErr(warning)
	}
	return cidr, nil
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
	userData := ec2.RenderUserData()
	// The size gate runs here so an outgrown toolchain fails the create before any
	// terraform invocation rather than partway through an apply.
	if err := ec2.ValidateUserDataSize(userData); err != nil {
		return launchedInstance{}, err
	}
	if err := ec2.WriteInstanceFile(b.MetadataDir, plan.name, userData); err != nil {
		return launchedInstance{}, err
	}

	terraform, err := b.applySharedInfrastructure(plan)
	if err != nil {
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

// applySharedInfrastructure runs init and apply over the shared working
// directory — every environment's instance file plus the VPC, security group,
// and key pair they share — and, only once the apply has returned without
// error, records the ingress CIDR it was given as the one the security group
// now holds. A failed apply leaves the earlier record in place, so the file
// never names an address that did not reach AWS. The runner is returned for a
// caller that goes on to read terraform's outputs.
func (b *EC2Backend) applySharedInfrastructure(plan environmentPlan) (*ec2.TerraformRunner, error) {
	terraform := ec2.NewTerraformRunner(b.Runner, plan.terraformConfig(b.MetadataDir), b.errOut())
	if err := terraform.Init(); err != nil {
		return nil, err
	}
	if err := terraform.Apply(plan.applyVariables()); err != nil {
		return nil, err
	}
	if err := ec2.PersistIngressCIDR(b.MetadataDir, plan.host.ingressCIDR); err != nil {
		return nil, err
	}
	return terraform, nil
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
	// own report is what names a failed module, or the warnings of a degraded
	// run that is otherwise ready.
	query := b.instanceQuery(publicDNS)

	if err := ec2.WaitForSSH(query, b.sleep()); err != nil {
		return err
	}

	b.print("Waiting for cloud-init...")
	notice, err := ec2.WaitForCloudInit(query, b.sleep())
	if err != nil {
		return err
	}
	if notice != "" {
		b.printErr(notice)
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

	if _, err := t.backend.applySharedInfrastructure(plan); err != nil {
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

	cidr, err := t.backend.resolveIngressCIDR(ec2.OpDestroy)
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

func (b *EC2Backend) printErr(message string) {
	_, _ = fmt.Fprintln(b.errOut(), message)
}

// Exec runs the command inside the instance's tmux session even though it is
// non-interactive, so the long-running command i2code starts keeps running when
// the connection drops and a re-run of the same command finds it again. A
// session already running a different command is left alone: nothing is run,
// and the error names both commands. Either way the exit code reported is the
// command's own, read from the session's status file, not the tmux client's.
func (b *EC2Backend) Exec(req ExecRequest) (int, error) {
	return b.withSession(req.ContainerName, func(target sessionTarget) (int, error) {
		if ec2.SessionExists(b.instanceSession(target.publicDNS, b.ExecFunc), target.session) {
			return b.rejoinSession(target, req.Args)
		}
		return b.startSession(target, req)
	})
}

// startSession runs the command in a fresh session. The session's status file
// is cleared first, so a status left by an earlier command can never be read as
// this one's.
func (b *EC2Backend) startSession(target sessionTarget, req ExecRequest) (int, error) {
	if err := b.instanceQuery(target.publicDNS).ClearExitStatus(target.session); err != nil {
		return 1, err
	}
	return b.runInSession(target, ec2.RemoteCommand{
		Workdir: ec2.RemoteRepoDir,
		EnvVars: req.EnvVars,
		Args:    ec2.BuildDetachableTmuxCommand(target.session, req.Args),
	})
}

// rejoinSession attaches to a session that is running the very command Exec
// was asked to run, streaming it until it ends, and refuses any other session
// rather than starting a second command beside it. The status file is left as
// the original command wrote it, so the reattached run reports the same status.
func (b *EC2Backend) rejoinSession(target sessionTarget, args []string) (int, error) {
	recorded, err := b.instanceQuery(target.publicDNS).RecordedCommand(target.session)
	if err != nil {
		return 1, err
	}
	if recorded != ec2.CommandRecord(args) {
		return 1, ec2.SessionBusyError(target.session, recorded, args)
	}

	b.printErr(fmt.Sprintf("reattaching to session '%s', which is already running: %s", target.session, recorded))
	return b.runInSession(target, ec2.RemoteCommand{Args: ec2.BuildAttachCommand(target.session)})
}

// runInSession carries cmd over the session transport and, once the tmux client
// has returned, reports the status the session's command recorded rather than
// the client's own. A transport error is returned as it is, so a connection
// that never reached the instance still buys the caller its address refresh.
func (b *EC2Backend) runInSession(target sessionTarget, cmd ec2.RemoteCommand) (int, error) {
	exitCode, err := b.execInSession()(b.MetadataDir, target.publicDNS, cmd)
	if err != nil {
		return exitCode, err
	}
	return b.instanceQuery(target.publicDNS).ReadExitStatus(target.session)
}

func (b *EC2Backend) instanceQuery(publicDNS string) ec2.InstanceQuery {
	return ec2.NewInstanceQuery(b.MetadataDir, publicDNS, b.capture())
}

// ExecInteractive runs the command inside the instance's tmux session, so a
// dropped connection leaves it running rather than killing it.
func (b *EC2Backend) ExecInteractive(req ExecRequest) (int, error) {
	return b.inSession(req.ContainerName, func(target sessionTarget) (int, error) {
		return b.ExecInteractiveFunc(b.MetadataDir, target.publicDNS, ec2.RemoteCommand{
			Workdir: ec2.RemoteRepoDir,
			EnvVars: req.EnvVars,
			Args:    ec2.BuildTmuxCommand(target.session, req.Args),
		})
	})
}

func (b *EC2Backend) OpenShell(req ExecRequest) (int, error) {
	return b.inSession(req.ContainerName, func(target sessionTarget) (int, error) {
		return ec2.OpenShell(b.instanceSession(target.publicDNS, b.ExecInteractiveFunc), target.session, req.EnvVars)
	})
}

// onInstance resolves where the environment can be reached from the metadata
// recorded at create time, so running a command costs no AWS or terraform call.
// Commands run from the repository create placed on the instance.
//
// The instance ID is immutable but the public DNS is not, so a connection that
// never reached the instance — as opposed to a command it ran and rejected —
// buys one lookup of the current address, one rewrite of metadata.json, and one
// further attempt. A stop and start recovers; a genuinely unreachable instance
// costs two attempts rather than a loop.
func (b *EC2Backend) onInstance(name string, run func(publicDNS string) (int, error)) (int, error) {
	store := ec2.NewMetadataStore(b.MetadataDir, name)
	meta, err := store.Read()
	if err != nil {
		return 1, err
	}
	if err := b.ensureHostIngress(*meta); err != nil {
		return 1, err
	}

	exitCode, err := run(meta.PublicDNS)
	if !errors.Is(err, ec2.ErrSSHConnect) {
		return exitCode, err
	}

	publicDNS, refreshErr := b.refreshPublicDNS(store, *meta)
	if refreshErr != nil {
		return exitCode, fmt.Errorf("%w; and its current address could not be looked up: %v", err, refreshErr)
	}
	b.printErr(fmt.Sprintf("instance moved to %s; retrying", publicDNS))
	return run(publicDNS)
}

// ensureHostIngress re-applies the shared SSH ingress rule when the host's
// public address is no longer the one the rule was last applied with, so a host
// that moved network can still reach its instances. An unchanged address costs
// no terraform or AWS call. A host whose address cannot be detected is warned
// and connected anyway, since the command might still work; a re-apply that
// fails is an error, because the connection it was clearing the way for would
// fail too.
func (b *EC2Backend) ensureHostIngress(meta ec2.Metadata) error {
	change, warning, err := ec2.IngressChanged(b.MetadataDir, b.CheckIPFunc)
	if err != nil {
		return err
	}
	if warning != "" {
		b.printErr(warning)
		return nil
	}
	if !change.Changed {
		return nil
	}

	b.printErr(fmt.Sprintf("host address changed from %s to %s; updating SSH ingress...", describePersistedCIDR(change.Persisted), change.Detected))
	plan, err := b.sharedInfrastructurePlan(meta.Region, change.Detected)
	if err != nil {
		return err
	}
	_, err = b.applySharedInfrastructure(plan)
	return err
}

func describePersistedCIDR(persisted string) string {
	if persisted == "" {
		return "(none recorded)"
	}
	return persisted
}

// sharedInfrastructurePlan is what re-applying the shared rule needs: the
// region create recorded, so a run needs no AWS_REGION of its own, the state
// bucket in it, and the same public key create applied.
func (b *EC2Backend) sharedInfrastructurePlan(region, ingressCIDR string) (environmentPlan, error) {
	bucket, err := b.EnsureBucketFunc(context.Background(), region)
	if err != nil {
		return environmentPlan{}, err
	}

	publicKey, err := b.EnsureKeypairFunc(b.MetadataDir)
	if err != nil {
		return environmentPlan{}, err
	}
	return environmentPlan{
		region: region,
		bucket: bucket,
		host:   hostState{publicKey: publicKey, ingressCIDR: ingressCIDR},
	}, nil
}

// refreshPublicDNS asks AWS where the recorded instance can be reached now and
// leaves metadata.json holding the answer, so the next command connects straight
// away without a lookup of its own.
func (b *EC2Backend) refreshPublicDNS(store *ec2.MetadataStore, meta ec2.Metadata) (string, error) {
	publicDNS, _, err := b.describeInstance()(context.Background(), meta.Region, meta.InstanceID)
	if err != nil {
		return "", err
	}

	meta.PublicDNS = publicDNS
	if err := store.Write(meta); err != nil {
		return "", err
	}
	return publicDNS, nil
}

// sessionTarget is the tmux session a command is about to join, on the address
// where its instance can currently be reached.
type sessionTarget struct {
	publicDNS string
	session   string
}

// withSession resolves the tmux session a command is about to join on the
// instance and hands it to run. The session is resolved inside the retried
// attempt, so a refreshed address is the one it is asked of.
func (b *EC2Backend) withSession(name string, run func(target sessionTarget) (int, error)) (int, error) {
	return b.onInstance(name, func(publicDNS string) (int, error) {
		session, err := b.sessionName(publicDNS)
		if err != nil {
			return 1, err
		}
		return run(sessionTarget{publicDNS: publicDNS, session: session})
	})
}

// inSession is withSession for the interactive commands, announcing a reattach
// first because tmux discards the command it was handed once the session
// already exists.
func (b *EC2Backend) inSession(name string, run func(target sessionTarget) (int, error)) (int, error) {
	return b.withSession(name, func(target sessionTarget) (int, error) {
		ec2.AnnounceReattach(b.errOut(), b.instanceSession(target.publicDNS, b.ExecFunc), target.session)
		return run(target)
	})
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
// It goes through onInstance like every other connecting operation, so it gets
// the host-address check and the address refresh; the exit code onInstance
// carries is meaningless for a copy and dropped.
func (b *EC2Backend) CopyCredentials(name string, credentials string) error {
	_, err := b.onInstance(name, func(publicDNS string) (int, error) {
		return 0, b.copyCredentials()(b.MetadataDir, publicDNS, credentials)
	})
	return err
}

func (b *EC2Backend) copyCredentials() ec2CopyCredentialsFunc {
	if b.CopyCredentialsFunc != nil {
		return b.CopyCredentialsFunc
	}
	return func(base, publicDNS, credentials string) error {
		return ec2.NewInstanceQuery(base, publicDNS, b.capture()).CopyClaudeCredentials(credentials)
	}
}
