package backend

import (
	"context"
	"errors"
	"strings"
	"testing"
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

func ec2BackendWithEnv(env map[string]string, spy *ensureBucketSpy) *EC2Backend {
	return &EC2Backend{
		MetadataDir: "/tmp/isolarium-test",
		LookupEnvFunc: func(key string) (string, bool) {
			value, ok := env[key]
			return value, ok
		},
		EnsureBucketFunc: spy.ensureBucket,
	}
}

func TestEC2Backend_Create_FailsWhenRegionUnset(t *testing.T) {
	spy := &ensureBucketSpy{name: "isolarium-tfstate-123456789012-us-west-2"}
	b := ec2BackendWithEnv(map[string]string{}, spy)

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
	b := ec2BackendWithEnv(map[string]string{"AWS_REGION": "us-west-2"}, spy)

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

func TestEC2Backend_Create_ReturnsBucketBootstrapError(t *testing.T) {
	accessDenied := errors.New("AccessDenied")
	spy := &ensureBucketSpy{err: accessDenied}
	b := ec2BackendWithEnv(map[string]string{"AWS_REGION": "us-west-2"}, spy)

	err := b.Create(CreateOptions{Name: "my-work"})

	if !errors.Is(err, accessDenied) {
		t.Fatalf("Create() error = %v, want it to wrap %v", err, accessDenied)
	}
}
