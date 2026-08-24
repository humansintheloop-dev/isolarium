//go:build ec2

// This file sorts immediately after lifecycle_ec2_test.go and immediately ahead
// of repo_ec2_test.go, which is where the stop and start it performs belongs:
// early enough that no test has yet built up tmux sessions a reboot would throw
// away, and late enough that the instance it reboots already exists. Every test
// after it works against the address the restart produced rather than the one
// create recorded, which adoptCurrentPublicDNS below is what keeps true.
package ec2_test

import (
	"context"
	"testing"
	"time"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// instanceStateTimeout is generous because a stop and a start are each minutes
// of AWS's time, not seconds.
const instanceStateTimeout = 10 * time.Minute

// TestEC2Instance_RecoversFromChangedDNS exercises the refresh-on-failure path
// against the only thing that really moves an instance's address: AWS itself.
// Stopping and starting through the SDK leaves metadata.json naming a host that
// no longer reaches the instance, so the echo below can only print anything by
// looking the instance up by its unchanged ID and rewriting the file.
func TestEC2Instance_RecoversFromChangedDNS(t *testing.T) {
	environment := sharedInstance(t)
	t.Cleanup(environment.adoptCurrentPublicDNS)

	staleDNS := environment.recordedPublicDNS()
	movedDNS := environment.stopAndStartOutOfBand()
	environment.assertAddressMoved(staleDNS, movedDNS)

	// The instance has to be answering at its new address before the recovery is
	// asked for, because the refresh buys exactly one retry — waiting out the boot
	// here keeps the test about the address change rather than about boot timing.
	environment.waitForSSHAt(movedDNS)
	environment.assertMetadataStillNames(staleDNS)

	environment.assertEchoWritesHelloToStdout()

	environment.assertMetadataWasRefreshedTo(movedDNS)
}

// stopAndStartOutOfBand takes the instance down and brings it back through the
// AWS API alone, which is what makes the new address a surprise: nothing in
// isolarium is told the instance moved.
func (e *ec2Environment) stopAndStartOutOfBand() string {
	e.t.Helper()

	ctx := context.Background()
	api := e.ec2API()

	if _, err := api.StopInstances(ctx, &awsec2.StopInstancesInput{InstanceIds: []string{e.instanceID}}); err != nil {
		e.t.Fatalf("stopping %s: %v", e.instanceID, err)
	}
	e.waitForInstanceStopped(ctx, api)

	if _, err := api.StartInstances(ctx, &awsec2.StartInstancesInput{InstanceIds: []string{e.instanceID}}); err != nil {
		e.t.Fatalf("starting %s: %v", e.instanceID, err)
	}
	e.waitForInstanceRunning(ctx, api)

	return e.currentPublicDNS()
}

func (e *ec2Environment) waitForInstanceStopped(ctx context.Context, api *awsec2.Client) {
	e.t.Helper()

	if err := awsec2.NewInstanceStoppedWaiter(api).Wait(ctx, e.describeSelfInput(), instanceStateTimeout); err != nil {
		e.t.Fatalf("waiting for %s to stop within %s: %v", e.instanceID, instanceStateTimeout, err)
	}
}

func (e *ec2Environment) waitForInstanceRunning(ctx context.Context, api *awsec2.Client) {
	e.t.Helper()

	if err := awsec2.NewInstanceRunningWaiter(api).Wait(ctx, e.describeSelfInput(), instanceStateTimeout); err != nil {
		e.t.Fatalf("waiting for %s to start within %s: %v", e.instanceID, instanceStateTimeout, err)
	}
}

func (e *ec2Environment) describeSelfInput() *awsec2.DescribeInstancesInput {
	return &awsec2.DescribeInstancesInput{InstanceIds: []string{e.instanceID}}
}

func (e *ec2Environment) currentPublicDNS() string {
	e.t.Helper()

	publicDNS, _, err := ec2.DescribeInstance(context.Background(), e.ec2API(), e.instanceID)
	if err != nil {
		e.t.Fatalf("describing %s after the restart: %v", e.instanceID, err)
	}
	return publicDNS
}

// assertAddressMoved refuses to let the test pass vacuously. AWS is free to hand
// a restarted instance the address it had before, and were it to do so the echo
// would succeed without any refresh having happened at all.
func (e *ec2Environment) assertAddressMoved(stale, moved string) {
	e.t.Helper()

	if moved == "" {
		e.t.Fatalf("instance %s has no public DNS after the restart", e.instanceID)
	}
	if moved == stale {
		e.t.Fatalf("instance %s came back on the same public DNS %s, so the refresh under test was never exercised",
			e.instanceID, stale)
	}
}

// assertMetadataStillNames pins the precondition the recovery needs: the file
// the CLI reads addresses the instance where it no longer is.
func (e *ec2Environment) assertMetadataStillNames(stale string) {
	e.t.Helper()

	if recorded := e.recordedPublicDNS(); recorded != stale {
		e.t.Fatalf("metadata.json holds %q going into the recovery, want the stale %q", recorded, stale)
	}
}

func (e *ec2Environment) assertMetadataWasRefreshedTo(moved string) {
	e.t.Helper()

	meta := e.recordedMetadata()
	if meta.PublicDNS != moved {
		e.t.Errorf("metadata.json public_dns after the recovery = %q, want %q", meta.PublicDNS, moved)
	}
	if meta.InstanceID != e.instanceID {
		e.t.Errorf("metadata.json instance_id after the recovery = %q, want the unchanged %q", meta.InstanceID, e.instanceID)
	}
}

func (e *ec2Environment) recordedPublicDNS() string {
	e.t.Helper()

	return e.recordedMetadata().PublicDNS
}

func (e *ec2Environment) recordedMetadata() ec2.Metadata {
	e.t.Helper()

	meta, err := ec2.NewMetadataStore(e.base, e.name).Read()
	if err != nil {
		e.t.Fatalf("reading metadata for %s: %v", e.name, err)
	}
	return *meta
}

// adoptCurrentPublicDNS hands the rest of the suite whatever address the
// instance ended up on, so that a failure part-way through this test does not
// cascade into every later test speaking to an address AWS has taken away.
// Destroying the instance however this test ends is already the suite's job —
// terminateSharedInstance runs once every test has returned, including a run
// abandoned here.
func (e *ec2Environment) adoptCurrentPublicDNS() {
	publicDNS, _, err := ec2.DescribeInstance(context.Background(), e.ec2API(), e.instanceID)
	if err != nil {
		e.t.Logf("could not re-read the address of %s after the DNS recovery test: %v", e.instanceID, err)
		return
	}
	e.publicDNS = publicDNS
}
