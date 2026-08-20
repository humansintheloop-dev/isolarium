package backend

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

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

func ec2BackendWithEnv(t *testing.T, env map[string]string, spy *ensureBucketSpy) *EC2Backend {
	t.Helper()
	return ec2BackendWithHostProvisioning(env, spy, newHostProvisioningSpy(), t.TempDir())
}

func ec2BackendWithHostProvisioning(env map[string]string, bucket *ensureBucketSpy, host *hostProvisioningSpy, metadataDir string) *EC2Backend {
	return &EC2Backend{
		MetadataDir: metadataDir,
		LookupEnvFunc: func(key string) (string, bool) {
			value, ok := env[key]
			return value, ok
		},
		EnsureBucketFunc:       bucket.ensureBucket,
		ExtractScaffoldingFunc: host.extractScaffolding,
		EnsureKeypairFunc:      host.ensureKeypair,
		DetectPublicIPFunc:     host.detectPublicIP,
	}
}

func TestEC2Backend_Create_FailsWhenRegionUnset(t *testing.T) {
	spy := &ensureBucketSpy{name: "isolarium-tfstate-123456789012-us-west-2"}
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
	spy := &ensureBucketSpy{name: "isolarium-tfstate-123456789012-us-west-2"}
	b := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := b.Create(CreateOptions{Name: "my-work"})

	if !spy.called {
		t.Fatal("Create() did not bootstrap the state bucket")
	}
	if spy.region != "us-west-2" {
		t.Errorf("EnsureBucketFunc received region %q, want %q", spy.region, "us-west-2")
	}
	if err == nil || !strings.Contains(err.Error(), "not yet implemented for --type ec2") {
		t.Errorf("Create() error = %v, want the not-yet-implemented error for the remaining steps", err)
	}
}

func TestEC2Backend_Create_ProvisionsHostStateAfterBucketBootstrap(t *testing.T) {
	bucket := &ensureBucketSpy{name: "isolarium-tfstate-123456789012-us-west-2"}
	host := newHostProvisioningSpy()
	metadataDir := t.TempDir()
	b := ec2BackendWithHostProvisioning(map[string]string{"AWS_REGION": "us-west-2"}, bucket, host, metadataDir)

	_ = b.Create(CreateOptions{Name: "my-work"})

	if got := strings.Join(host.calls, ","); got != "scaffold,keypair,detect" {
		t.Errorf("host provisioning order = %q, want %q", got, "scaffold,keypair,detect")
	}
	if host.scaffoldBase != metadataDir || host.keypairBase != metadataDir {
		t.Errorf("host provisioning bases = %q and %q, want %q", host.scaffoldBase, host.keypairBase, metadataDir)
	}
	assertPersistedIngressCIDR(t, metadataDir, "203.0.113.7/32")
}

func TestEC2Backend_Create_FailsWhenPublicIPDetectionFails(t *testing.T) {
	offline := errors.New("dial tcp: no route to host")
	host := newHostProvisioningSpy()
	host.detectErr = offline
	metadataDir := t.TempDir()
	b := ec2BackendWithHostProvisioning(
		map[string]string{"AWS_REGION": "us-west-2"},
		&ensureBucketSpy{name: "isolarium-tfstate-123456789012-us-west-2"},
		host,
		metadataDir,
	)

	err := b.Create(CreateOptions{Name: "my-work"})

	if !errors.Is(err, offline) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, offline)
	}
	if _, statErr := os.Stat(ec2.TfvarsPath(metadataDir)); statErr == nil {
		t.Errorf("Create() persisted an ingress CIDR despite failed detection")
	}
}

func TestEC2Backend_Create_ReturnsScaffoldingError(t *testing.T) {
	readOnly := errors.New("permission denied")
	host := newHostProvisioningSpy()
	host.scaffoldErr = readOnly
	b := ec2BackendWithHostProvisioning(
		map[string]string{"AWS_REGION": "us-west-2"},
		&ensureBucketSpy{name: "isolarium-tfstate-123456789012-us-west-2"},
		host,
		t.TempDir(),
	)

	err := b.Create(CreateOptions{Name: "my-work"})

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

func TestEC2Backend_Create_ReturnsBucketBootstrapError(t *testing.T) {
	accessDenied := errors.New("AccessDenied")
	spy := &ensureBucketSpy{err: accessDenied}
	b := ec2BackendWithEnv(t, map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := b.Create(CreateOptions{Name: "my-work"})

	if !errors.Is(err, accessDenied) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, accessDenied)
	}
}
