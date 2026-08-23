package backend

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

type ensureBucketSpy struct {
	called bool
	region string
	name   string
	err    error
}

func (s *ensureBucketSpy) ensureBucket(ctx context.Context, region string) (string, error) {
	s.called = true
	s.region = region
	return s.name, s.err
}

// hostProvisioningSpy records the order in which Create provisions host state.
type hostProvisioningSpy struct {
	calls           []string
	scaffoldBase    string
	keypairBase     string
	publicKey       string
	detectedAddress string
	scaffoldErr     error
	keypairErr      error
	detectErr       error
}

func (s *hostProvisioningSpy) extractScaffolding(base string) error {
	s.calls = append(s.calls, "scaffold")
	s.scaffoldBase = base
	return s.scaffoldErr
}

func (s *hostProvisioningSpy) ensureKeypair(base string) (string, error) {
	s.calls = append(s.calls, "keypair")
	s.keypairBase = base
	return s.publicKey, s.keypairErr
}

// checkIP stands in for the public-IP lookup service, so the spy drives the same
// detection code the real backend runs rather than short-circuiting it.
func (s *hostProvisioningSpy) checkIP(string) (string, error) {
	s.calls = append(s.calls, "detect")
	return s.detectedAddress + "\n", s.detectErr
}

const ec2SpyDetectedCIDR = "203.0.113.7/32"

func newHostProvisioningSpy() *hostProvisioningSpy {
	return &hostProvisioningSpy{
		publicKey:       "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIspy isolarium",
		detectedAddress: "203.0.113.7",
	}
}

// ec2RemoteSpy stands in for the SSH transport during Create, recording every
// command the instance was asked to run. A command named in neverSucceeds always
// exits non-zero, which is how an unreachable instance and a stalled cloud-init
// are reproduced; cloudInitWarnings, when set, is the degraded report the
// cloud-init probe answers with.
type ec2RemoteSpy struct {
	commands          []ec2.RemoteCommand
	base              string
	publicDNS         string
	neverSucceeds     string
	rejects           string
	cloudInitWarnings string
}

func (s *ec2RemoteSpy) exec(base, publicDNS string, cmd ec2.RemoteCommand) (int, error) {
	s.commands = append(s.commands, cmd)
	s.base = base
	s.publicDNS = publicDNS
	if s.neverSucceeds != "" && cmd.Args[0] == s.neverSucceeds {
		return 255, nil
	}
	if s.rejects != "" && strings.Contains(renderRemoteCommand(cmd), s.rejects) {
		return 1, nil
	}
	return 0, nil
}

// capture stands in for the same transport when the caller reads the command's
// output as well as its exit code, which is how the readiness probes run.
func (s *ec2RemoteSpy) capture(base, publicDNS string, cmd ec2.RemoteCommand) (string, int, error) {
	exitCode, err := s.exec(base, publicDNS, cmd)
	if s.cloudInitWarnings != "" && cmd.Args[0] == "cloud-init" {
		return s.cloudInitWarnings, cloudInitDegradedExit, nil
	}
	return "", exitCode, err
}

// cloudInitDegradedExit is what `cloud-init status` exits with when provisioning
// finished with warnings and no failed module.
const cloudInitDegradedExit = 2

func (s *ec2RemoteSpy) ranCommand(marker string) bool {
	for _, cmd := range s.commands {
		if strings.Contains(renderRemoteCommand(cmd), marker) {
			return true
		}
	}
	return false
}

// renderRemoteCommand reproduces what the instance's shell sees, so an assertion
// can be written the way the command reads on the wire.
func renderRemoteCommand(cmd ec2.RemoteCommand) string {
	if cmd.Workdir == "" {
		return strings.Join(cmd.Args, " ")
	}
	return "cd " + cmd.Workdir + " && " + strings.Join(cmd.Args, " ")
}

const (
	ec2SpyOwner  = "humansintheloop-dev"
	ec2SpyRepo   = "isolarium"
	ec2SpyBranch = "idea/ec2-isolation-type"
	ec2SpyToken  = "ghs_exampleclonetoken"
)

// repositorySourceSpy records whether Create resolved the repository, since
// resolving it pushes a branch and mints a token.
type repositorySourceSpy struct {
	hostDir string
	calls   int
	err     error
}

func (s *repositorySourceSpy) resolve() (ec2.RepositorySpec, error) {
	s.calls++
	if s.err != nil {
		return ec2.RepositorySpec{}, s.err
	}
	return ec2.RepositorySpec{
		Owner:       ec2SpyOwner,
		Repo:        ec2SpyRepo,
		Branch:      ec2SpyBranch,
		Token:       ec2SpyToken,
		HostDir:     s.hostDir,
		AuthorEmail: "chris@example.com",
		AuthorName:  "Chris Richardson",
	}, nil
}

// newRepositorySourceSpy backs the source with a host checkout carrying both
// project config files, so every copy step has something to carry.
func newRepositorySourceSpy(t *testing.T) *repositorySourceSpy {
	t.Helper()

	hostDir := t.TempDir()
	writeHostProjectFile(t, filepath.Join(hostDir, ".claude", "settings.local.json"), `{"permissions":{}}`)
	writeHostProjectFile(t, filepath.Join(hostDir, "CLAUDE.md"), "# Project Guidelines\n")
	return &repositorySourceSpy{hostDir: hostDir}
}

func writeHostProjectFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

const (
	ec2SpyBucket     = "isolarium-tfstate-123456789012-us-west-2"
	ec2SpyInstanceID = "i-0123456789abcdef0"
	ec2SpyPublicDNS  = "ec2-203-0-113-7.compute-1.amazonaws.com"
)

var ec2SpyCreatedAt = time.Date(2026, 8, 19, 14, 3, 21, 0, time.UTC)

func ec2TerraformOutputJSON(name string) string {
	return `{"instance_id_` + name + `": {"value": "` + ec2SpyInstanceID + `"},` +
		`"public_dns_` + name + `": {"value": "` + ec2SpyPublicDNS + `"}}`
}

// ec2BackendFixture assembles an EC2Backend whose every collaborator is a spy,
// so Create can be driven with no AWS account, no network, and no terraform.
type ec2BackendFixture struct {
	env         map[string]string
	bucket      *ensureBucketSpy
	host        *hostProvisioningSpy
	remote      *ec2RemoteSpy
	repository  *repositorySourceSpy
	metadataDir string
	workDir     string
	runner      command.Runner
}

func (f ec2BackendFixture) backend() *EC2Backend {
	return &EC2Backend{
		MetadataDir: f.metadataDir,
		Runner:      f.runner,
		NowFunc:     func() time.Time { return ec2SpyCreatedAt },
		LookupEnvFunc: func(key string) (string, bool) {
			value, ok := f.env[key]
			return value, ok
		},
		EnsureBucketFunc:       f.bucket.ensureBucket,
		ExtractScaffoldingFunc: f.host.extractScaffolding,
		EnsureKeypairFunc:      f.host.ensureKeypair,
		CheckIPFunc:            f.host.checkIP,
		ExecFunc:               f.remote.exec,
		CaptureFunc:            f.remote.capture,
		SleepFunc:              func(time.Duration) {},
	}
}

// createOptions names the environment every fixture creates and hands Create the
// spied repository source.
func (f ec2BackendFixture) createOptions() CreateOptions {
	return CreateOptions{Name: "my-work", WorkDirectory: f.workDir, Repository: f.repository.resolve}
}

func ec2BackendWithEnv(t *testing.T, env map[string]string, spy *ensureBucketSpy) ec2BackendFixture {
	t.Helper()

	return ec2BackendFixture{
		env:         env,
		bucket:      spy,
		host:        newHostProvisioningSpy(),
		remote:      &ec2RemoteSpy{},
		repository:  newRepositorySourceSpy(t),
		metadataDir: t.TempDir(),
		workDir:     t.TempDir(),
		runner:      ec2FakeTerraform(t),
	}
}

func ec2FakeTerraform(t *testing.T) *command.FakeRunner {
	t.Helper()

	return ec2FakeTerraformAtVersion(t, supportedTerraformVersion)
}

// supportedTerraformVersion is what the fake terraform reports unless a test is
// about the version gate itself.
const supportedTerraformVersion = "1.10.5"

func ec2FakeTerraformAtVersion(t *testing.T, version string) *command.FakeRunner {
	t.Helper()

	runner := ec2TerraformAnsweringOnlyVersion(t, version)
	runner.OnCommand("terraform").Returns(ec2TerraformOutputJSON("my-work"))
	return runner
}

// ec2TerraformAnsweringOnlyVersion clears the version gate and nothing else, so
// a test that expects Create to give up before applying fails loudly if it
// reaches terraform at all.
func ec2TerraformAnsweringOnlyVersion(t *testing.T, version string) *command.FakeRunner {
	t.Helper()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("terraform", "version", "-json").Returns(`{"terraform_version":"` + version + `"}`)
	return runner
}

func ec2RegionalFixture(t *testing.T, host *hostProvisioningSpy, runner command.Runner) ec2BackendFixture {
	t.Helper()

	return ec2BackendFixture{
		env:         map[string]string{"AWS_REGION": "us-west-2"},
		bucket:      &ensureBucketSpy{name: ec2SpyBucket},
		host:        host,
		remote:      &ec2RemoteSpy{},
		repository:  newRepositorySourceSpy(t),
		metadataDir: t.TempDir(),
		workDir:     t.TempDir(),
		runner:      runner,
	}
}

func TestEC2Backend_Create_FailsWhenRegionUnset(t *testing.T) {
	spy := &ensureBucketSpy{name: ec2SpyBucket}
	f := ec2BackendWithEnv(t, map[string]string{}, spy)

	err := f.backend().Create(f.createOptions())

	if err == nil {
		t.Fatal("Create() returned nil error when AWS_REGION is unset")
	}
	if !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("Create() error = %q, want it to mention AWS_REGION", err.Error())
	}
	if spy.called {
		t.Error("Create() bootstrapped the state bucket despite the missing region")
	}
	if f.repository.calls != 0 {
		t.Error("Create() pushed the branch and minted a clone token for an environment it never launched")
	}
}

func TestEC2Backend_Create_BootstrapsStateBucketWithResolvedRegion(t *testing.T) {
	spy := &ensureBucketSpy{name: ec2SpyBucket}
	f := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := f.backend().Create(f.createOptions())

	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !spy.called {
		t.Fatal("Create() did not bootstrap the state bucket")
	}
	if spy.region != "us-west-2" {
		t.Errorf("EnsureBucketFunc received region %q, want %q", spy.region, "us-west-2")
	}
}

func TestEC2Backend_Create_ProvisionsHostStateAfterBucketBootstrap(t *testing.T) {
	host := newHostProvisioningSpy()
	fixture := ec2RegionalFixture(t, host, ec2FakeTerraform(t))

	_ = fixture.backend().Create(fixture.createOptions())

	if got := strings.Join(host.calls, ","); got != "scaffold,keypair,detect" {
		t.Errorf("host provisioning order = %q, want %q", got, "scaffold,keypair,detect")
	}
	assertHostProvisionedUnder(t, host, fixture.metadataDir)
	assertPersistedIngressCIDR(t, fixture.metadataDir, ec2SpyDetectedCIDR)
}

func assertHostProvisionedUnder(t *testing.T, host *hostProvisioningSpy, metadataDir string) {
	t.Helper()

	if host.scaffoldBase != metadataDir {
		t.Errorf("ExtractScaffoldingFunc received base %q, want %q", host.scaffoldBase, metadataDir)
	}
	if host.keypairBase != metadataDir {
		t.Errorf("EnsureKeypairFunc received base %q, want %q", host.keypairBase, metadataDir)
	}
}

func TestEC2Backend_Create_FailsWhenPublicIPDetectionFails(t *testing.T) {
	offline := errors.New("dial tcp: no route to host")
	host := newHostProvisioningSpy()
	host.detectErr = offline
	fixture := ec2RegionalFixture(t, host, ec2TerraformAnsweringOnlyVersion(t, supportedTerraformVersion))

	err := fixture.backend().Create(fixture.createOptions())

	if !errors.Is(err, offline) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, offline)
	}
	if _, statErr := os.Stat(ec2.TfvarsPath(fixture.metadataDir)); statErr == nil {
		t.Errorf("Create() persisted an ingress CIDR despite failed detection")
	}
}

func TestEC2Backend_Create_ReturnsScaffoldingError(t *testing.T) {
	readOnly := errors.New("permission denied")
	host := newHostProvisioningSpy()
	host.scaffoldErr = readOnly
	fixture := ec2RegionalFixture(t, host, ec2TerraformAnsweringOnlyVersion(t, supportedTerraformVersion))

	err := fixture.backend().Create(fixture.createOptions())

	if !errors.Is(err, readOnly) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, readOnly)
	}
	if strings.Join(host.calls, ",") != "scaffold" {
		t.Errorf("host provisioning continued past the scaffolding failure: %v", host.calls)
	}
}

func assertPersistedIngressCIDR(t *testing.T, metadataDir, want string) {
	t.Helper()

	got, err := ec2.ReadPersistedIngressCIDR(metadataDir)
	if err != nil {
		t.Fatalf("reading the persisted ingress CIDR: %v", err)
	}
	if got != want {
		t.Errorf("persisted ingress CIDR = %q, want %q", got, want)
	}
}

func TestEC2Backend_Create_LaunchesInstance(t *testing.T) {
	host := newHostProvisioningSpy()
	runner := ec2FakeTerraform(t)
	fixture := ec2RegionalFixture(t, host, runner)

	if err := fixture.backend().Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertInstanceFileDescribesOnlyTheInstance(t, fixture.metadataDir)
	assertInstanceFileCarriesCloudInitUserData(t, fixture.metadataDir)
	assertTerraformInvocations(t, runner, fixture.metadataDir, host.publicKey)
	assertRecordedMetadata(t, fixture.metadataDir)
}

func TestEC2Backend_Create_WaitsForCloudInitOnTheNewInstance(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))

	if err := fixture.backend().Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !fixture.remote.ranCommand("cloud-init status --wait") {
		t.Fatalf("Create() ran %v, want it to wait for cloud-init", fixture.remote.commands)
	}
	if fixture.remote.base != fixture.metadataDir {
		t.Errorf("the SSH transport received base %q, want %q", fixture.remote.base, fixture.metadataDir)
	}
	if fixture.remote.publicDNS != ec2SpyPublicDNS {
		t.Errorf("the SSH transport received public DNS %q, want the applied instance's %q",
			fixture.remote.publicDNS, ec2SpyPublicDNS)
	}
}

func TestEC2Backend_Create_FailsWhenCloudInitNeverFinishes(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	fixture.remote.neverSucceeds = "cloud-init"

	err := fixture.backend().Create(fixture.createOptions())

	if err == nil {
		t.Fatal("Create() returned nil error for an instance whose cloud-init never finished")
	}
	if want := "instance did not finish cloud-init within 15m0s"; !strings.Contains(err.Error(), want) {
		t.Errorf("Create() error = %q, want it to contain %q", err, want)
	}
	if fixture.remote.ranCommand("git clone") {
		t.Error("Create() cloned the repository onto an instance that never finished provisioning")
	}
	assertRecordedMetadata(t, fixture.metadataDir)
}

func TestEC2Backend_Create_TimesOutWaitingForSSH(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	fixture.remote.neverSucceeds = "true"

	err := fixture.backend().Create(fixture.createOptions())

	if err == nil {
		t.Fatal("Create() returned nil error for an instance that never became reachable")
	}
	if want := "instance did not become reachable over SSH within 5m0s"; !strings.Contains(err.Error(), want) {
		t.Errorf("Create() error = %q, want it to contain %q", err, want)
	}
	if fixture.remote.ranCommand("cloud-init status --wait") {
		t.Error("Create() waited for cloud-init on an instance it could not reach")
	}
	assertRecordedMetadata(t, fixture.metadataDir)
}

func TestEC2Backend_Create_PlacesRepository(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))

	if err := fixture.backend().Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertRemoteCommandSequence(t, fixture.remote.commands, []string{
		"true",
		"cloud-init status --wait",
		"git clone --branch " + ec2SpyBranch +
			" https://x-access-token:" + ec2SpyToken + "@github.com/" + ec2SpyOwner + "/" + ec2SpyRepo + ".git repo",
		"cd " + ec2.RemoteRepoDir + " && git remote set-url origin https://github.com/" +
			ec2SpyOwner + "/" + ec2SpyRepo + ".git",
		"cd " + ec2.RemoteRepoDir + " && git config user.email 'chris+i2code@example.com'",
		"cd " + ec2.RemoteRepoDir + " && git config user.name 'Chris Richardson - i2code'",
		"> " + ec2.RemoteRepoDir + "/.claude/settings.local.json",
		"> " + ec2.RemoteRepoDir + "/CLAUDE.md",
	})
	assertCloneTokenNeverReachesTheInstanceDisk(t, fixture.remote.commands)
	assertRecordedRepository(t, fixture.metadataDir)
}

func TestEC2Backend_Create_ReportsProgressThroughEachStage(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	out := &bytes.Buffer{}
	b := fixture.backend()
	b.Out = out

	if err := b.Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	want := "Creating EC2 instance...\nWaiting for cloud-init...\nCloning repository...\n"
	if got := out.String(); got != want {
		t.Errorf("Create() printed %q, want %q", got, want)
	}
}

// A cloud-init run that finished degraded — warnings logged, no module failed —
// is an instance that is ready and has something to mention. Create goes on to
// clone and relays the warnings on stderr, where they cannot be mistaken for
// the command's own output.
func TestEC2Backend_Create_RelaysCloudInitWarningsAndCarriesOn(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	fixture.remote.cloudInitWarnings = "status: done\nextended_status: degraded done\nerrors: []\nrecoverable_errors:\nWARNING:\n\t- Calling 'http://[fd00:ec2::254]/latest/api/token' failed\n"
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	b := fixture.backend()
	b.Out = out
	b.ErrWriter = errOut

	if err := b.Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v, want nil for a degraded run with no failed module", err)
	}

	if !fixture.remote.ranCommand("git clone") {
		t.Error("Create() did not clone the repository onto an instance whose cloud-init finished with warnings only")
	}
	for _, want := range []string{"degraded", "fd00:ec2::254"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("Create() wrote %q to stderr, want it to relay %q", errOut.String(), want)
		}
	}
	if strings.Contains(out.String(), "fd00:ec2::254") {
		t.Errorf("Create() wrote the cloud-init warnings to stdout:\n%s", out.String())
	}
}

func assertRemoteCommandSequence(t *testing.T, commands []ec2.RemoteCommand, markers []string) {
	t.Helper()

	if len(commands) != len(markers) {
		t.Fatalf("ran %d remote commands, want %d: %v", len(commands), len(markers), commands)
	}
	for i, marker := range markers {
		if got := renderRemoteCommand(commands[i]); !strings.Contains(got, marker) {
			t.Errorf("remote command %d = %q, want it to contain %q", i, got, marker)
		}
	}
}

// assertCloneTokenNeverReachesTheInstanceDisk pins the one place the token is
// allowed to appear: the argument of the single git clone that consumes it.
func assertCloneTokenNeverReachesTheInstanceDisk(t *testing.T, commands []ec2.RemoteCommand) {
	t.Helper()

	var carrying []string
	for _, cmd := range commands {
		if rendered := renderRemoteCommand(cmd); strings.Contains(rendered, ec2SpyToken) {
			carrying = append(carrying, rendered)
		}
	}

	if len(carrying) != 1 {
		t.Fatalf("%d remote commands carry the clone token, want exactly 1: %v", len(carrying), carrying)
	}
	if !strings.HasPrefix(carrying[0], "git clone ") {
		t.Errorf("the command carrying the clone token is %q, want it to be the git clone", carrying[0])
	}
}

func assertRecordedRepository(t *testing.T, metadataDir string) {
	t.Helper()

	meta, err := ec2.NewMetadataStore(metadataDir, "my-work").Read()
	if err != nil {
		t.Fatalf("reading the recorded metadata: %v", err)
	}
	got := meta.Owner + "/" + meta.Repo + "@" + meta.Branch
	if want := ec2SpyOwner + "/" + ec2SpyRepo + "@" + ec2SpyBranch; got != want {
		t.Errorf("metadata repository = %q, want %q", got, want)
	}
}

func assertInstanceFileCarriesCloudInitUserData(t *testing.T, metadataDir string) {
	t.Helper()

	content := readInstanceFile(t, metadataDir)
	for _, want := range []string{"user_data = <<-USER_DATA", "#cloud-config", "- tmux"} {
		if !strings.Contains(content, want) {
			t.Errorf("instance-my-work.tf does not contain %q", want)
		}
	}
}

func readInstanceFile(t *testing.T, metadataDir string) string {
	t.Helper()

	path := ec2.InstanceFilePath(metadataDir, "my-work")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func TestEC2Backend_Create_RefusesExistingInstanceFile(t *testing.T) {
	runner := ec2TerraformAnsweringOnlyVersion(t, supportedTerraformVersion)
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), runner)
	if err := ec2.WriteInstanceFile(fixture.metadataDir, "my-work", ""); err != nil {
		t.Fatalf("seeding the instance file: %v", err)
	}

	err := fixture.backend().Create(fixture.createOptions())

	if err == nil {
		t.Fatal("Create() returned nil error for an environment that already exists")
	}
	want := "instance-my-work.tf already exists; run isolarium destroy --type ec2 --name my-work first"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Create() error = %q, want it to contain %q", err.Error(), want)
	}
	assertOnlyProbedTerraformVersion(t, runner)
}

// assertOnlyProbedTerraformVersion allows the version gate, which runs before
// Create can know the environment already exists, and nothing beyond it.
func assertOnlyProbedTerraformVersion(t *testing.T, runner *command.FakeRunner) {
	t.Helper()

	calls := runner.Calls()
	if len(calls) != 1 || strings.Join(calls[0], " ") != "terraform version -json" {
		t.Errorf("Create() ran %v, want only the terraform version probe", calls)
	}
}

func assertInstanceFileDescribesOnlyTheInstance(t *testing.T, metadataDir string) {
	t.Helper()

	content := readInstanceFile(t, metadataDir)
	for _, want := range []string{
		`resource "aws_instance" "my-work"`,
		"data.aws_ssm_parameter.ubuntu_ami.value",
		`instance_type               = "t3.large"`,
		"aws_key_pair.isolarium.key_name",
		"aws_security_group.isolarium.id",
		"aws_subnet.isolarium.id",
		"associate_public_ip_address = true",
		"volume_size           = 50",
		`volume_type           = "gp3"`,
		"encrypted             = true",
		"delete_on_termination = true",
		`Name = "my-work"`,
		`output "instance_id_my-work"`,
		`output "public_dns_my-work"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("instance-my-work.tf does not contain %q", want)
		}
	}
	for _, forbidden := range []string{"ingress", "cidr_blocks"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("instance-my-work.tf contains %q, want ingress to stay in the shared security group", forbidden)
		}
	}
	if got := strings.Count(content, `resource "aws_instance"`); got != 1 {
		t.Errorf("instance-my-work.tf declares %d aws_instance resources, want exactly 1", got)
	}
}

func assertTerraformInvocations(t *testing.T, runner *command.FakeRunner, metadataDir, publicKey string) {
	t.Helper()

	terraformDir := ec2.TerraformDir(metadataDir)
	want := []string{
		"terraform version -json",
		strings.Join([]string{
			"terraform", "-chdir=" + terraformDir, "init",
			"-backend-config=bucket=" + ec2SpyBucket,
			"-backend-config=key=" + ec2.StateKey,
			"-backend-config=region=us-west-2",
		}, " "),
		strings.Join([]string{
			"terraform", "-chdir=" + terraformDir, "apply",
			"-auto-approve", "-input=false", "-lock-timeout=120s",
			"-var=ingress_cidr=203.0.113.7/32",
			"-var=public_key=" + publicKey,
			"-var=region=us-west-2",
		}, " "),
		strings.Join([]string{"terraform", "-chdir=" + terraformDir, "output", "-json"}, " "),
	}

	calls := runner.Calls()
	if len(calls) != len(want) {
		t.Fatalf("recorded %d terraform invocations, want %d: %v", len(calls), len(want), calls)
	}
	for i, wantCall := range want {
		if got := strings.Join(calls[i], " "); got != wantCall {
			t.Errorf("invocation %d =\n  %s\nwant\n  %s", i, got, wantCall)
		}
	}
}

func assertRecordedMetadata(t *testing.T, metadataDir string) {
	t.Helper()

	meta, err := ec2.NewMetadataStore(metadataDir, "my-work").Read()
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.Join(metadataDir, "my-work", "ec2", "metadata.json"), err)
	}
	if meta.InstanceID != ec2SpyInstanceID {
		t.Errorf("metadata instance_id = %q, want %q", meta.InstanceID, ec2SpyInstanceID)
	}
	if meta.PublicDNS != ec2SpyPublicDNS {
		t.Errorf("metadata public_dns = %q, want %q", meta.PublicDNS, ec2SpyPublicDNS)
	}
	if meta.Region != "us-west-2" {
		t.Errorf("metadata region = %q, want %q", meta.Region, "us-west-2")
	}
	if !meta.CreatedAt.Equal(ec2SpyCreatedAt) {
		t.Errorf("metadata created_at = %v, want %v", meta.CreatedAt, ec2SpyCreatedAt)
	}
}

// ec2DestroyFixture is a backend whose environment "my-work" has already been
// created — instance file, initialised Terraform directory, persisted ingress
// CIDR, known_hosts entry, and metadata all on disk — so Destroy has real host
// state to tear down.
type ec2DestroyFixture struct {
	backend *EC2Backend
	host    *hostProvisioningSpy
	runner  *command.FakeRunner
	out     *bytes.Buffer
	errOut  *bytes.Buffer
	base    string
}

func ec2BackendWithCreatedEnvironment(t *testing.T) ec2DestroyFixture {
	t.Helper()

	host := newHostProvisioningSpy()
	runner := command.NewFakeRunner(t)
	runner.OnCommand("terraform").Returns("")
	runner.OnCommand("ssh-keygen").Returns("")

	fixture := ec2RegionalFixture(t, host, runner)
	seedCreatedEnvironment(t, fixture.metadataDir)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	b := fixture.backend()
	b.Out = out
	b.ErrWriter = errOut
	return ec2DestroyFixture{backend: b, host: host, runner: runner, out: out, errOut: errOut, base: fixture.metadataDir}
}

func seedCreatedEnvironment(t *testing.T, base string) {
	t.Helper()

	if err := ec2.WriteInstanceFile(base, "my-work", ""); err != nil {
		t.Fatalf("seeding the instance file: %v", err)
	}
	if err := ec2.PersistIngressCIDR(base, ec2SpyPersistedCIDR); err != nil {
		t.Fatalf("seeding the persisted ingress CIDR: %v", err)
	}
	seedInitialisedTerraformDir(t, base)
	seedKnownHosts(t, base)
	seedRecordedInstance(t, base)
}

func seedInitialisedTerraformDir(t *testing.T, base string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(ec2.TerraformDir(base), ".terraform"), 0755); err != nil {
		t.Fatalf("seeding the initialised terraform directory: %v", err)
	}
}

func seedKnownHosts(t *testing.T, base string) {
	t.Helper()

	if err := os.WriteFile(ec2.KnownHostsPath(base), []byte(ec2SpyPublicDNS+" ssh-ed25519 AAAA\n"), 0600); err != nil {
		t.Fatalf("seeding %s: %v", ec2.KnownHostsPath(base), err)
	}
}

const ec2SpyPersistedCIDR = "198.51.100.9/32"

func TestEC2Destroy_BackendRemovesInstanceAndHostState(t *testing.T) {
	f := ec2BackendWithCreatedEnvironment(t)

	if err := f.backend.Destroy("my-work"); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	assertAbsent(t, "instance file", ec2.InstanceFilePath(f.base, "my-work"))
	assertAbsent(t, "metadata directory", filepath.Join(f.base, "my-work", "ec2"))
	assertDestroyInvocations(t, f)
	assertPersistedIngressCIDR(t, f.base, ec2SpyDetectedCIDR)
}

func assertAbsent(t *testing.T, label, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s %s still exists after Destroy()", label, path)
	}
}

func assertDestroyInvocations(t *testing.T, f ec2DestroyFixture) {
	t.Helper()

	want := []string{
		strings.Join([]string{
			"terraform", "-chdir=" + ec2.TerraformDir(f.base), "apply",
			"-auto-approve", "-input=false", "-lock-timeout=120s",
			"-var=ingress_cidr=203.0.113.7/32",
			"-var=public_key=" + f.host.publicKey,
			"-var=region=us-west-2",
		}, " "),
		strings.Join([]string{
			"ssh-keygen", "-R", ec2SpyPublicDNS, "-f", ec2.KnownHostsPath(f.base),
		}, " "),
	}

	calls := f.runner.Calls()
	if len(calls) != len(want) {
		t.Fatalf("recorded %d invocations, want %d: %v", len(calls), len(want), calls)
	}
	for i, wantCall := range want {
		if got := strings.Join(calls[i], " "); got != wantCall {
			t.Errorf("invocation %d =\n  %s\nwant\n  %s", i, got, wantCall)
		}
	}
}

func TestEC2Destroy_BackendIsIdempotent(t *testing.T) {
	f := ec2BackendWithCreatedEnvironment(t)
	if err := f.backend.Destroy("my-work"); err != nil {
		t.Fatalf("first Destroy() error = %v", err)
	}
	invocationsAfterFirstDestroy := len(f.runner.Calls())
	f.out.Reset()

	if err := f.backend.Destroy("my-work"); err != nil {
		t.Fatalf("second Destroy() error = %v, want nil", err)
	}

	if got := f.out.String(); !strings.Contains(got, "no EC2 environment to destroy") {
		t.Errorf("second Destroy() printed %q, want it to contain %q", got, "no EC2 environment to destroy")
	}
	if got := len(f.runner.Calls()); got != invocationsAfterFirstDestroy {
		t.Errorf("second Destroy() ran %v, want no further invocation", f.runner.Calls()[invocationsAfterFirstDestroy:])
	}
}

func TestEC2Backend_Destroy_FallsBackToPersistedCIDR(t *testing.T) {
	f := ec2BackendWithCreatedEnvironment(t)
	f.host.detectErr = errors.New("dial tcp: no route to host")

	if err := f.backend.Destroy("my-work"); err != nil {
		t.Fatalf("Destroy() error = %v, want detection failure not to block teardown", err)
	}

	assertAppliedIngressCIDR(t, f.runner, ec2SpyPersistedCIDR)
	assertContainsAll(t, "the warning on stderr", f.errOut.String(),
		"warning: public IP detection failed",
		"dial tcp: no route to host",
		ec2SpyPersistedCIDR,
	)
	assertAbsent(t, "instance file", ec2.InstanceFilePath(f.base, "my-work"))
}

func assertContainsAll(t *testing.T, name, content string, wanted ...string) {
	t.Helper()

	for _, want := range wanted {
		if !strings.Contains(content, want) {
			t.Errorf("%s = %q, want it to contain %q", name, content, want)
		}
	}
}

func assertContainsNone(t *testing.T, name, content string, unwanted ...string) {
	t.Helper()

	for _, forbidden := range unwanted {
		if strings.Contains(content, forbidden) {
			t.Errorf("%s = %q, want it not to contain %q", name, content, forbidden)
		}
	}
}

func assertAppliedIngressCIDR(t *testing.T, runner *command.FakeRunner, want string) {
	t.Helper()

	got, ok := appliedIngressCIDR(runner)
	if !ok {
		t.Fatalf("no apply carried an ingress CIDR: %v", runner.Calls())
	}
	if got != want {
		t.Errorf("apply used ingress CIDR %q, want %q", got, want)
	}
}

func appliedIngressCIDR(runner *command.FakeRunner) (string, bool) {
	const flag = "-var=ingress_cidr="

	for _, call := range runner.Calls() {
		for _, arg := range call {
			if strings.HasPrefix(arg, flag) {
				return strings.TrimPrefix(arg, flag), true
			}
		}
	}
	return "", false
}

func TestEC2Destroy_BackendFailsWhenNeitherDetectionNorPersistenceYieldsACIDR(t *testing.T) {
	f := ec2BackendWithCreatedEnvironment(t)
	f.host.detectErr = errors.New("dial tcp: no route to host")
	if err := os.Remove(ec2.TfvarsPath(f.base)); err != nil {
		t.Fatalf("removing the persisted ingress CIDR: %v", err)
	}

	err := f.backend.Destroy("my-work")

	if err == nil {
		t.Fatal("Destroy() returned nil error with no detected and no persisted ingress CIDR")
	}
	if len(f.runner.Calls()) != 0 {
		t.Errorf("Destroy() ran %v, want no invocation", f.runner.Calls())
	}
	if !ec2.InstanceFileExists(f.base, "my-work") {
		t.Error("Destroy() removed the instance file despite failing before the apply")
	}
}

// An interrupted create can leave instance-<name>.tf behind with no
// metadata.json, and destroy is the documented way out of that state.
func TestEC2Destroy_BackendTearsDownAnEnvironmentThatNeverRecordedMetadata(t *testing.T) {
	f := ec2BackendWithCreatedEnvironment(t)
	if err := ec2.NewMetadataStore(f.base, "my-work").Cleanup(); err != nil {
		t.Fatalf("removing the recorded metadata: %v", err)
	}

	if err := f.backend.Destroy("my-work"); err != nil {
		t.Fatalf("Destroy() error = %v, want an environment with no metadata to be destroyable", err)
	}

	assertAbsent(t, "instance file", ec2.InstanceFilePath(f.base, "my-work"))
	if got := len(f.runner.Calls()); got != 1 {
		t.Errorf("recorded %d invocations, want only the apply: %v", got, f.runner.Calls())
	}
}

// copyCredentialsSpy records the instance a credential copy was aimed at and the
// blob it was handed.
type copyCredentialsSpy struct {
	called      bool
	base        string
	publicDNS   string
	credentials string
	err         error
}

func (s *copyCredentialsSpy) copy(base, publicDNS, credentials string) error {
	s.called = true
	s.base = base
	s.publicDNS = publicDNS
	s.credentials = credentials
	return s.err
}

// instanceCredentialSpy answers the credential read with a canned instance-side
// blob and counts every command the copy issues, so a copy that decided to leave
// the instance alone can be told apart from one that wrote.
type instanceCredentialSpy struct {
	output  string
	calls   int
	command ec2.RemoteCommand
}

func (s *instanceCredentialSpy) capture(base, publicDNS string, cmd ec2.RemoteCommand) (string, int, error) {
	s.calls++
	s.command = cmd
	return s.output, 0, nil
}

func TestEC2CopyCredentials_BackendAimsAtTheRecordedInstance(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	spy := &copyCredentialsSpy{}
	f.backend.CopyCredentialsFunc = spy.copy

	if err := f.backend.CopyCredentials("my-work", `{"claudeAiOauth":{"expiresAt":1000}}`); err != nil {
		t.Fatalf("CopyCredentials() error = %v, want nil", err)
	}

	if !spy.called {
		t.Fatal("CopyCredentials() never reached the copy")
	}
	if spy.base != f.backend.MetadataDir {
		t.Errorf("CopyCredentials() used base %q, want %q", spy.base, f.backend.MetadataDir)
	}
	if spy.publicDNS != ec2SpyPublicDNS {
		t.Errorf("CopyCredentials() used public DNS %q, want the one recorded in metadata.json (%q)", spy.publicDNS, ec2SpyPublicDNS)
	}
	if spy.credentials != `{"claudeAiOauth":{"expiresAt":1000}}` {
		t.Errorf("CopyCredentials() passed credentials %q, want the host blob", spy.credentials)
	}
}

func TestEC2CopyCredentials_BackendFailsWhenTheEnvironmentWasNeverCreated(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	spy := &copyCredentialsSpy{}
	f.backend.CopyCredentialsFunc = spy.copy

	if err := f.backend.CopyCredentials("never-created", "{}"); err == nil {
		t.Fatal("CopyCredentials() returned nil error for an environment with no metadata.json")
	}
	if spy.called {
		t.Error("CopyCredentials() reached the copy despite the missing metadata")
	}
}

// TestEC2CopyCredentials_BackendReadsTheInstanceOverSSHByDefault proves the
// unwired backend carries out the real conditional copy rather than panicking on
// a nil function: an instance whose credentials outlive the host's is read and
// then left alone.
func TestEC2CopyCredentials_BackendReadsTheInstanceOverSSHByDefault(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	capture := &instanceCredentialSpy{output: `{"claudeAiOauth":{"expiresAt":2000}}`}
	f.backend.CaptureFunc = capture.capture

	if err := f.backend.CopyCredentials("my-work", `{"claudeAiOauth":{"expiresAt":1000}}`); err != nil {
		t.Fatalf("CopyCredentials() error = %v, want nil", err)
	}

	assertArgsEqual(t, "credential read", capture.command.Args, []string{"cat", "~/.claude/.credentials.json", "2>/dev/null"})
	if capture.calls != 1 {
		t.Errorf("CopyCredentials() issued %d commands, want only the read when the instance is fresher", capture.calls)
	}
}

// describeInstanceStub stands in for the AWS lookup GetState makes, recording
// which instance it was asked about and answering with a fixed AWS state.
type describeInstanceStub struct {
	called     bool
	region     string
	instanceID string
	state      string
	err        error
}

func (s *describeInstanceStub) describe(ctx context.Context, region, instanceID string) (string, string, error) {
	s.called = true
	s.region = region
	s.instanceID = instanceID
	return ec2SpyPublicDNS, s.state, s.err
}

func TestEC2State_BackendMapsEveryAWSState(t *testing.T) {
	mappings := []struct {
		awsState string
		want     string
	}{
		{"running", "running"},
		{"stopped", "stopped"},
		{"stopping", "stopped"},
		{"pending", "pending"},
		{"shutting-down", "none"},
		{"terminated", "none"},
		{"a-state-aws-has-not-invented-yet", "unknown"},
	}

	for _, mapping := range mappings {
		t.Run(mapping.awsState, func(t *testing.T) {
			f := ec2BackendWithRecordedInstance(t, 0)
			describe := &describeInstanceStub{state: mapping.awsState}
			f.backend.DescribeInstanceFunc = describe.describe

			state := f.backend.GetState("my-work")

			if state != mapping.want {
				t.Errorf("GetState() = %q for AWS state %q, want %q", state, mapping.awsState, mapping.want)
			}
			assertDescribedRecordedInstance(t, describe)
		})
	}
}

func assertDescribedRecordedInstance(t *testing.T, describe *describeInstanceStub) {
	t.Helper()

	if describe.instanceID != ec2SpyInstanceID {
		t.Errorf("GetState() described instance %q, want the one recorded in metadata.json (%q)", describe.instanceID, ec2SpyInstanceID)
	}
	if describe.region != "us-west-2" {
		t.Errorf("GetState() described in region %q, want the one recorded in metadata.json (%q)", describe.region, "us-west-2")
	}
}

func TestEC2State_BackendReportsNoneWhenMetadataIsAbsent(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	describe := &describeInstanceStub{state: "running"}
	f.backend.DescribeInstanceFunc = describe.describe

	state := f.backend.GetState("never-created")

	if state != "none" {
		t.Errorf("GetState() = %q for an environment with no metadata.json, want %q", state, "none")
	}
	if describe.called {
		t.Error("GetState() called AWS despite the missing metadata")
	}
}

func TestEC2State_BackendReportsUnknownWhenTheAWSCallFails(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)
	describe := &describeInstanceStub{err: errors.New("NoCredentialProviders: no valid providers in chain")}
	f.backend.DescribeInstanceFunc = describe.describe

	state := f.backend.GetState("my-work")

	if state != "unknown" {
		t.Errorf("GetState() = %q when the AWS call failed, want %q", state, "unknown")
	}
}

func TestEC2Backend_Create_ReturnsBucketBootstrapError(t *testing.T) {
	accessDenied := errors.New("AccessDenied")
	spy := &ensureBucketSpy{name: ec2SpyBucket, err: accessDenied}
	f := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := f.backend().Create(f.createOptions())

	if !errors.Is(err, accessDenied) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, accessDenied)
	}
}

func TestEC2Backend_Create_RejectsOldTerraform(t *testing.T) {
	spy := &ensureBucketSpy{name: ec2SpyBucket}
	f := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)
	f.runner = ec2FakeTerraformAtVersion(t, "1.9.8")

	err := f.backend().Create(f.createOptions())

	if err == nil {
		t.Fatal("Create() returned nil error when terraform is older than 1.10")
	}
	want := "terraform 1.10.0 or later is required for --type ec2 (found 1.9.8)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Create() error = %q, want it to contain %q", err.Error(), want)
	}
	if spy.called {
		t.Error("Create() bootstrapped the state bucket despite the unsupported terraform")
	}
	if f.repository.calls != 0 {
		t.Error("Create() pushed the branch and minted a clone token for an environment it never launched")
	}
}

func TestEC2Backend_Create_AcceptsTerraform110(t *testing.T) {
	spy := &ensureBucketSpy{name: ec2SpyBucket}
	f := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)
	f.runner = ec2FakeTerraformAtVersion(t, "1.10.5")

	err := f.backend().Create(f.createOptions())

	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !spy.called {
		t.Error("Create() did not get past the version gate to bootstrap the state bucket")
	}
}
