package backend

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// ec2HostIngressFixture is a recorded instance whose terraform fake answers
// every invocation, with stderr captured, so a run whose host address changed
// can re-apply the shared infrastructure and the notice it prints can be read.
type ec2HostIngressFixture struct {
	ec2ExecFixture
	errOut *bytes.Buffer
}

func ec2BackendReachedFromAnotherAddress(t *testing.T) ec2HostIngressFixture {
	t.Helper()

	f := ec2BackendWithIdleInstance(t, 0)
	persistIngressCIDR(t, f.backend.MetadataDir, ec2SpyPersistedCIDR)
	f.terraform.OnCommand("terraform").Returns("")
	errOut := &bytes.Buffer{}
	f.backend.ErrWriter = errOut
	return ec2HostIngressFixture{ec2ExecFixture: f, errOut: errOut}
}

func runHello(b *EC2Backend) (int, error) {
	return b.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"echo", "hello"}})
}

func TestEC2HostIngress_ExecMakesNoTerraformCallWhenTheHostAddressIsUnchanged(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)
	errOut := &bytes.Buffer{}
	f.backend.ErrWriter = errOut

	if _, err := runHello(f.backend); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	if !f.session.called {
		t.Fatal("Exec() did not reach the session SSH transport")
	}
	assertNoRemoteInfrastructureCalls(t, f.bucket, f.terraform)
	assertContainsNone(t, "stderr", errOut.String(), "host address changed", "warning")
}

func TestEC2HostIngress_ExecReappliesTheIngressRuleWhenTheHostAddressChanged(t *testing.T) {
	f := ec2BackendReachedFromAnotherAddress(t)
	var terraformCallsWhenConnecting int
	f.backend.ExecInSessionFunc = func(base, publicDNS string, cmd ec2.RemoteCommand) (int, error) {
		terraformCallsWhenConnecting = len(f.terraform.Calls())
		return f.session.exec(base, publicDNS, cmd)
	}

	exitCode, err := runHello(f.backend)

	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exec() exit code = %d, want 0", exitCode)
	}
	assertSharedInfrastructureApplied(t, f.ec2ExecFixture)
	if terraformCallsWhenConnecting != 2 {
		t.Errorf("Exec() had run %d terraform invocations when it reached the SSH transport, want init and apply first", terraformCallsWhenConnecting)
	}
	assertPersistedIngressCIDR(t, f.backend.MetadataDir, ec2SpyDetectedCIDR)
	assertContainsAll(t, "stderr", f.errOut.String(),
		"host address changed from "+ec2SpyPersistedCIDR+" to "+ec2SpyDetectedCIDR+"; updating SSH ingress...",
	)
}

// assertSharedInfrastructureApplied pins that the re-apply is the same init and
// apply create runs, over the shared working directory, with the region from
// metadata.json and the host's own public key, and nothing else — no output
// read, no version probe.
func assertSharedInfrastructureApplied(t *testing.T, f ec2ExecFixture) {
	t.Helper()

	terraformDir := ec2.TerraformDir(f.backend.MetadataDir)
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
			"-var=ingress_cidr=" + ec2SpyDetectedCIDR,
			"-var=public_key=" + newHostProvisioningSpy().publicKey,
			"-var=region=us-west-2",
		}, " "),
	}

	calls := f.terraform.Calls()
	if len(calls) != len(want) {
		t.Fatalf("recorded %d terraform invocations, want %d: %v", len(calls), len(want), calls)
	}
	for i, wantCall := range want {
		if got := strings.Join(calls[i], " "); got != wantCall {
			t.Errorf("invocation %d =\n  %s\nwant\n  %s", i, got, wantCall)
		}
	}
}

// The region comes from metadata.json, so a run needs no AWS_REGION in its
// environment even when it has to re-apply.
func TestEC2HostIngress_ReapplyTakesTheRegionFromMetadata(t *testing.T) {
	f := ec2BackendReachedFromAnotherAddress(t)
	f.backend.LookupEnvFunc = func(string) (string, bool) { return "", false }

	if _, err := runHello(f.backend); err != nil {
		t.Fatalf("Exec() error = %v, want the region to come from metadata.json", err)
	}

	assertSharedInfrastructureApplied(t, f.ec2ExecFixture)
	if f.bucket.region != "us-west-2" {
		t.Errorf("the state bucket was resolved for region %q, want the recorded %q", f.bucket.region, "us-west-2")
	}
}

func TestEC2HostIngress_ExecFailsWithoutRunningWhenTheReapplyFails(t *testing.T) {
	f := ec2BackendReachedFromAnotherAddress(t)
	applyFails := errors.New("Error: acquiring the state lock")
	f.terraform.OnCommand("terraform", "-chdir="+ec2.TerraformDir(f.backend.MetadataDir), "apply").Fails(applyFails)

	_, err := runHello(f.backend)

	if !errors.Is(err, applyFails) {
		t.Fatalf("Exec() error = %v, want it to wrap %v", err, applyFails)
	}
	if f.session.called {
		t.Error("Exec() reached the SSH transport although the ingress rule could not be re-applied")
	}
	assertPersistedIngressCIDR(t, f.backend.MetadataDir, ec2SpyPersistedCIDR)
}

func TestEC2HostIngress_ExecWarnsAndConnectsWhenDetectionFails(t *testing.T) {
	f := ec2BackendReachedFromAnotherAddress(t)
	f.backend.CheckIPFunc = func(string) (string, error) { return "", errors.New("dial tcp: no route to host") }

	if _, err := runHello(f.backend); err != nil {
		t.Fatalf("Exec() error = %v, want a detection failure not to block the command", err)
	}

	if !f.session.called {
		t.Fatal("Exec() did not reach the session SSH transport")
	}
	assertNoRemoteInfrastructureCalls(t, f.bucket, f.terraform)
	assertContainsAll(t, "stderr", f.errOut.String(), "warning: public IP detection failed", "dial tcp: no route to host")
	assertPersistedIngressCIDR(t, f.backend.MetadataDir, ec2SpyPersistedCIDR)
}

func TestEC2HostIngress_ShellReappliesTheIngressRuleWhenTheHostAddressChanged(t *testing.T) {
	f := ec2BackendReachedFromAnotherAddress(t)

	if _, err := f.backend.OpenShell(ExecRequest{ContainerName: "my-work"}); err != nil {
		t.Fatalf("OpenShell() error = %v", err)
	}

	if !f.interactive.called {
		t.Fatal("OpenShell() did not reach the interactive SSH transport")
	}
	assertSharedInfrastructureApplied(t, f.ec2ExecFixture)
	assertPersistedIngressCIDR(t, f.backend.MetadataDir, ec2SpyDetectedCIDR)
}

func TestEC2HostIngress_CopyCredentialsReappliesTheIngressRuleWhenTheHostAddressChanged(t *testing.T) {
	f := ec2BackendReachedFromAnotherAddress(t)
	spy := &copyCredentialsSpy{}
	var terraformCallsWhenCopying int
	f.backend.CopyCredentialsFunc = func(base, publicDNS, credentials string) error {
		terraformCallsWhenCopying = len(f.terraform.Calls())
		return spy.copy(base, publicDNS, credentials)
	}

	if err := f.backend.CopyCredentials("my-work", `{"claudeAiOauth":{"expiresAt":1000}}`); err != nil {
		t.Fatalf("CopyCredentials() error = %v", err)
	}

	if !spy.called {
		t.Fatal("CopyCredentials() never reached the copy")
	}
	assertSharedInfrastructureApplied(t, f.ec2ExecFixture)
	if terraformCallsWhenCopying != 2 {
		t.Errorf("CopyCredentials() had run %d terraform invocations when it reached the copy, want init and apply first", terraformCallsWhenCopying)
	}
	assertPersistedIngressCIDR(t, f.backend.MetadataDir, ec2SpyDetectedCIDR)
	assertContainsAll(t, "stderr", f.errOut.String(), "host address changed from "+ec2SpyPersistedCIDR+" to "+ec2SpyDetectedCIDR)
}

// CopyCredentials goes through onInstance, so a moved instance gets the same
// address refresh and retry the other connecting operations get.
func TestEC2HostIngress_CopyCredentialsRefreshesTheAddressOnConnectFailure(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 0)
	describe := &ec2RefreshStub{publicDNS: ec2MovedPublicDNS}
	f.backend.DescribeInstanceFunc = describe.describe
	var attempts []string
	f.backend.CopyCredentialsFunc = func(base, publicDNS, credentials string) error {
		attempts = append(attempts, publicDNS)
		if publicDNS == ec2SpyPublicDNS {
			return ec2ConnectFailure()
		}
		return nil
	}

	if err := f.backend.CopyCredentials("my-work", "{}"); err != nil {
		t.Fatalf("CopyCredentials() error = %v, want the retry at the refreshed address to have succeeded", err)
	}

	if strings.Join(attempts, " ") != ec2SpyPublicDNS+" "+ec2MovedPublicDNS {
		t.Errorf("CopyCredentials() attempted %v, want the recorded address and then the refreshed one", attempts)
	}
	assertRecordedPublicDNS(t, f.backend.MetadataDir, ec2MovedPublicDNS)
}
