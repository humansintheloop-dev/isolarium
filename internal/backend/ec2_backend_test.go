package backend

import (
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
	calls        []string
	scaffoldBase string
	keypairBase  string
	publicKey    string
	detectedCIDR string
	scaffoldErr  error
	keypairErr   error
	detectErr    error
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

func (s *hostProvisioningSpy) detectPublicIP() (string, error) {
	s.calls = append(s.calls, "detect")
	return s.detectedCIDR, s.detectErr
}

func newHostProvisioningSpy() *hostProvisioningSpy {
	return &hostProvisioningSpy{
		publicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIspy isolarium",
		detectedCIDR: "203.0.113.7/32",
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
	metadataDir string
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
		DetectPublicIPFunc:     f.host.detectPublicIP,
	}
}

func ec2BackendWithEnv(t *testing.T, env map[string]string, spy *ensureBucketSpy) *EC2Backend {
	t.Helper()

	return ec2BackendFixture{
		env:         env,
		bucket:      spy,
		host:        newHostProvisioningSpy(),
		metadataDir: t.TempDir(),
		runner:      ec2FakeTerraform(t),
	}.backend()
}

func ec2FakeTerraform(t *testing.T) *command.FakeRunner {
	t.Helper()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("terraform").Returns(ec2TerraformOutputJSON("my-work"))
	return runner
}

func ec2RegionalFixture(t *testing.T, host *hostProvisioningSpy, runner command.Runner) ec2BackendFixture {
	t.Helper()

	return ec2BackendFixture{
		env:         map[string]string{"AWS_REGION": "us-west-2"},
		bucket:      &ensureBucketSpy{name: ec2SpyBucket},
		host:        host,
		metadataDir: t.TempDir(),
		runner:      runner,
	}
}

func TestEC2Backend_Create_FailsWhenRegionUnset(t *testing.T) {
	spy := &ensureBucketSpy{name: ec2SpyBucket}
	b := ec2BackendWithEnv(t, map[string]string{}, spy)

	err := b.Create(CreateOptions{Name: "my-work"})

	if err == nil {
		t.Fatal("Create() returned nil error when AWS_REGION is unset")
	}
	if !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("Create() error = %q, want it to mention AWS_REGION", err.Error())
	}
	if spy.called {
		t.Error("Create() bootstrapped the state bucket despite the missing region")
	}
}

func TestEC2Backend_Create_BootstrapsStateBucketWithResolvedRegion(t *testing.T) {
	spy := &ensureBucketSpy{name: ec2SpyBucket}
	b := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := b.Create(CreateOptions{Name: "my-work"})

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

	_ = fixture.backend().Create(CreateOptions{Name: "my-work"})

	if got := strings.Join(host.calls, ","); got != "scaffold,keypair,detect" {
		t.Errorf("host provisioning order = %q, want %q", got, "scaffold,keypair,detect")
	}
	assertHostProvisionedUnder(t, host, fixture.metadataDir)
	assertPersistedIngressCIDR(t, fixture.metadataDir, "203.0.113.7/32")
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
	fixture := ec2RegionalFixture(t, host, command.NewFakeRunner(t))

	err := fixture.backend().Create(CreateOptions{Name: "my-work"})

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
	fixture := ec2RegionalFixture(t, host, command.NewFakeRunner(t))

	err := fixture.backend().Create(CreateOptions{Name: "my-work"})

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

	if err := fixture.backend().Create(CreateOptions{Name: "my-work"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertInstanceFileDescribesOnlyTheInstance(t, fixture.metadataDir)
	assertTerraformInvocations(t, runner, fixture.metadataDir, host.publicKey)
	assertRecordedMetadata(t, fixture.metadataDir)
}

func TestEC2Backend_Create_RefusesExistingInstanceFile(t *testing.T) {
	runner := command.NewFakeRunner(t)
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), runner)
	if err := ec2.WriteInstanceFile(fixture.metadataDir, "my-work", ""); err != nil {
		t.Fatalf("seeding the instance file: %v", err)
	}

	err := fixture.backend().Create(CreateOptions{Name: "my-work"})

	if err == nil {
		t.Fatal("Create() returned nil error for an environment that already exists")
	}
	want := "instance-my-work.tf already exists; run isolarium destroy --type ec2 --name my-work first"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Create() error = %q, want it to contain %q", err.Error(), want)
	}
	if len(runner.Calls()) != 0 {
		t.Errorf("Create() ran %v, want no terraform invocation", runner.Calls())
	}
}

func assertInstanceFileDescribesOnlyTheInstance(t *testing.T, metadataDir string) {
	t.Helper()

	path := ec2.InstanceFilePath(metadataDir, "my-work")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	content := string(data)
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

func TestEC2Backend_Create_ReturnsBucketBootstrapError(t *testing.T) {
	accessDenied := errors.New("AccessDenied")
	spy := &ensureBucketSpy{name: ec2SpyBucket, err: accessDenied}
	b := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := b.Create(CreateOptions{Name: "my-work"})

	if !errors.Is(err, accessDenied) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, accessDenied)
	}
}
