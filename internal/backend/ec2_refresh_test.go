package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// ec2MovedPublicDNS is where a stop and start leaves an instance that
// metadata.json still records at ec2SpyPublicDNS.
const ec2MovedPublicDNS = "ec2-198-51-100-9.compute-1.amazonaws.com"

func ec2ConnectFailure() error {
	return fmt.Errorf("%w: ssh exited 255", ec2.ErrSSHConnect)
}

// ec2RemoteOutcome is one scripted answer from the SSH transport.
type ec2RemoteOutcome struct {
	exitCode int
	err      error
}

// ec2RetrySpy stands in for the SSH transport across a refresh, recording the
// address every attempt aimed at and answering from a script whose last entry
// repeats for any further attempt.
type ec2RetrySpy struct {
	attempts []string
	script   []ec2RemoteOutcome
}

func (s *ec2RetrySpy) exec(base, publicDNS string, cmd ec2.RemoteCommand) (int, error) {
	s.attempts = append(s.attempts, publicDNS)
	outcome := s.script[len(s.script)-1]
	if len(s.attempts) <= len(s.script) {
		outcome = s.script[len(s.attempts)-1]
	}
	return outcome.exitCode, outcome.err
}

// ec2RefreshStub stands in for the DescribeInstances lookup a refresh makes,
// counting the lookups and answering with the address the instance moved to.
type ec2RefreshStub struct {
	calls       int
	instanceIDs []string
	regions     []string
	publicDNS   string
	err         error
}

func (s *ec2RefreshStub) describe(ctx context.Context, region, instanceID string) (string, string, error) {
	s.calls++
	s.instanceIDs = append(s.instanceIDs, instanceID)
	s.regions = append(s.regions, region)
	return s.publicDNS, "running", s.err
}

// ec2RefreshFixture is a backend whose environment "my-work" was created at
// ec2SpyPublicDNS, with the SSH transport and the AWS lookup both scripted so a
// connect failure and the refresh it provokes can be counted.
type ec2RefreshFixture struct {
	backend  *EC2Backend
	ssh      *ec2RetrySpy
	describe *ec2RefreshStub
}

func ec2BackendAnswering(t *testing.T, script ...ec2RemoteOutcome) ec2RefreshFixture {
	t.Helper()

	f := ec2BackendWithRecordedInstance(t, 0)
	ssh := &ec2RetrySpy{script: script}
	describe := &ec2RefreshStub{publicDNS: ec2MovedPublicDNS}
	f.backend.ExecFunc = ssh.exec
	f.backend.DescribeInstanceFunc = describe.describe
	return ec2RefreshFixture{backend: f.backend, ssh: ssh, describe: describe}
}

func (f ec2RefreshFixture) exec() (int, error) {
	return f.backend.Exec(ExecRequest{ContainerName: "my-work", Args: []string{"echo", "hello"}})
}

func TestEC2Backend_Exec_RefreshesMetadataOnConnectFailure(t *testing.T) {
	f := ec2BackendAnswering(t,
		ec2RemoteOutcome{exitCode: 255, err: ec2ConnectFailure()},
		ec2RemoteOutcome{exitCode: 0},
	)

	exitCode, err := f.exec()

	if err != nil {
		t.Fatalf("Exec() error = %v, want the retry to have succeeded", err)
	}
	if exitCode != 0 {
		t.Errorf("Exec() exit code = %d, want 0", exitCode)
	}
	assertRefreshedOnce(t, f.describe)
	assertSSHAttempts(t, f.ssh, ec2SpyPublicDNS, ec2MovedPublicDNS)
	assertRecordedPublicDNS(t, f.backend.MetadataDir, ec2MovedPublicDNS)
}

func TestEC2Backend_Exec_DoesNotRefreshOnNonZeroExit(t *testing.T) {
	f := ec2BackendAnswering(t, ec2RemoteOutcome{exitCode: 42})

	exitCode, err := f.exec()

	if err != nil {
		t.Fatalf("Exec() error = %v, want a remote non-zero exit to be no error at all", err)
	}
	if exitCode != 42 {
		t.Errorf("Exec() exit code = %d, want the remote exit code 42 verbatim", exitCode)
	}
	if f.describe.calls != 0 {
		t.Errorf("Exec() made %d AWS lookups for a command the instance ran and rejected, want 0", f.describe.calls)
	}
	assertSSHAttempts(t, f.ssh, ec2SpyPublicDNS)
	assertRecordedPublicDNS(t, f.backend.MetadataDir, ec2SpyPublicDNS)
}

func TestEC2Backend_Exec_RetriesAtMostOnce(t *testing.T) {
	f := ec2BackendAnswering(t, ec2RemoteOutcome{exitCode: 255, err: ec2ConnectFailure()})

	_, err := f.exec()

	if !errors.Is(err, ec2.ErrSSHConnect) {
		t.Fatalf("Exec() error = %v, want the connect failure to survive the retry", err)
	}
	assertRefreshedOnce(t, f.describe)
	assertSSHAttempts(t, f.ssh, ec2SpyPublicDNS, ec2MovedPublicDNS)
}

func assertRefreshedOnce(t *testing.T, describe *ec2RefreshStub) {
	t.Helper()

	if describe.calls != 1 {
		t.Fatalf("Exec() made %d AWS lookups, want exactly 1", describe.calls)
	}
	if describe.instanceIDs[0] != ec2SpyInstanceID {
		t.Errorf("Exec() looked up instance %q, want the one recorded in metadata.json (%q)", describe.instanceIDs[0], ec2SpyInstanceID)
	}
	if describe.regions[0] != "us-west-2" {
		t.Errorf("Exec() looked up in region %q, want the one recorded in metadata.json (%q)", describe.regions[0], "us-west-2")
	}
}

func assertSSHAttempts(t *testing.T, ssh *ec2RetrySpy, want ...string) {
	t.Helper()

	if strings.Join(ssh.attempts, " ") != strings.Join(want, " ") {
		t.Errorf("Exec() attempted SSH against %v, want %v", ssh.attempts, want)
	}
}

func assertRecordedPublicDNS(t *testing.T, metadataDir, want string) {
	t.Helper()

	meta, err := ec2.NewMetadataStore(metadataDir, "my-work").Read()
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	if meta.PublicDNS != want {
		t.Errorf("metadata.json public_dns = %q, want %q", meta.PublicDNS, want)
	}
	if meta.InstanceID != ec2SpyInstanceID {
		t.Errorf("metadata.json instance_id = %q, want it left alone at %q", meta.InstanceID, ec2SpyInstanceID)
	}
}
