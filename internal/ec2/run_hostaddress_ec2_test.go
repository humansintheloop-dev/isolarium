//go:build ec2

// The file name is load-bearing. It sorts after run_ec2_test.go, which leaves
// the shared tmux session free, and ahead of tmux_ec2_test.go, whose tests
// deliberately leave a process running in that session — which the runs here
// need free, the same as the run tests do.
package ec2_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	// wrongHostCIDR is a TEST-NET-2 address (RFC 5737), so the rule it is
	// applied to admits neither the host nor anyone else.
	wrongHostCIDR = "198.51.100.1/32"

	hostAddressChangedNotice = "host address changed from"
	securityGroupName        = "isolarium"
	sshPort                  = 22
	refusingTerraformMode    = 0o755
)

// TestEC2Run_ReappliesSSHIngressWhenTheHostAddressChanged proves, against the
// real security group, what a host that moved network relies on. The shared
// infrastructure is applied with a CIDR that is not the host's and the same
// CIDR is recorded in isolarium.auto.tfvars, so the host is locked out and the
// record agrees with AWS — exactly the state a host that changed address is in.
// A `run` then announces the change, re-applies the rule for the address it
// detected, and runs the command; a second `run` finds nothing changed and
// makes no terraform call at all.
func TestEC2Run_ReappliesSSHIngressWhenTheHostAddressChanged(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()
	hostCIDR := detectedHostCIDR(t)

	environment.lockTheHostOutOfSSH()
	environment.assertSSHIngressIsPinnedTo(wrongHostCIDR)
	environment.assertPersistedIngressCIDRIs(wrongHostCIDR)

	first := environment.runEchoOK(binaryEnvironment())
	first.assertPrintedOKAndExitedZero()
	first.assertAnnouncedTheAddressChange(wrongHostCIDR, hostCIDR)
	environment.assertSSHIngressIsPinnedTo(hostCIDR)
	environment.assertPersistedIngressCIDRIs(hostCIDR)

	refusingTerraform, invocations := refuseTerraform(t)
	second := environment.runEchoOK(refusingTerraform)
	second.assertPrintedOKAndExitedZero()
	second.assertAnnouncedNoAddressChange()
	assertTerraformWasNotCalled(t, invocations)
}

// detectedHostCIDR is the address the binary will detect too: both sides ask
// checkip.amazonaws.com from the same host.
func detectedHostCIDR(t *testing.T) string {
	t.Helper()

	cidr, err := ec2.DetectPublicIP(ec2.DefaultHTTPGet)
	if err != nil {
		t.Fatalf("detecting the host's public address: %v", err)
	}
	return cidr
}

// lockTheHostOutOfSSH puts the account and the host's record into the state a
// moved host is in: the security group admits an address that is not the
// host's, and isolarium.auto.tfvars says so. The apply is the product's own,
// over the working directory create initialised, so the group genuinely names
// the wrong host rather than the test only pretending it does.
func (e *ec2Environment) lockTheHostOutOfSSH() {
	e.t.Helper()

	e.t.Cleanup(e.restoreSSHIngressIfStillLockedOut)
	e.applySharedInfrastructureWithIngress(wrongHostCIDR)
	if err := ec2.PersistIngressCIDR(e.base, wrongHostCIDR); err != nil {
		e.t.Fatalf("recording %s as the applied ingress CIDR: %v", wrongHostCIDR, err)
	}
}

func (e *ec2Environment) applySharedInfrastructureWithIngress(cidr string) {
	e.t.Helper()

	publicKey, err := ec2.EnsureKeypair(e.base)
	if err != nil {
		e.t.Fatalf("reading the suite's public key: %v", err)
	}
	config := ec2.TerraformConfig{Base: e.base, Bucket: e.stateBucketName(), Region: e.region}
	terraform := ec2.NewTerraformRunner(command.ExecRunner{}, config, os.Stderr)
	if err := terraform.Init(); err != nil {
		e.t.Fatalf("initialising the shared working directory: %v", err)
	}
	variables := map[string]string{"ingress_cidr": cidr, "public_key": publicKey, "region": e.region}
	if err := terraform.Apply(variables); err != nil {
		e.t.Fatalf("applying the shared infrastructure with ingress_cidr=%s: %v", cidr, err)
	}
}

// restoreSSHIngressIfStillLockedOut keeps a failure part-way through this test
// from cascading into every later test's connection being refused. A run that
// got as far as the re-apply has already put the host's address back.
func (e *ec2Environment) restoreSSHIngressIfStillLockedOut() {
	persisted, err := ec2.ReadPersistedIngressCIDR(e.base)
	if err != nil || persisted != wrongHostCIDR {
		return
	}
	hostCIDR, err := ec2.DetectPublicIP(ec2.DefaultHTTPGet)
	if err != nil {
		e.t.Logf("could not detect the host address to restore SSH ingress after the test: %v", err)
		return
	}
	e.applySharedInfrastructureWithIngress(hostCIDR)
	if err := ec2.PersistIngressCIDR(e.base, hostCIDR); err != nil {
		e.t.Logf("could not record %s after restoring SSH ingress: %v", hostCIDR, err)
	}
}

// assertSSHIngressIsPinnedTo reads the rule from AWS rather than from the file
// that records it, so "the persisted record agrees with AWS" is checked against
// both sides. It is fatal because everything after it reasons from this state.
func (e *ec2Environment) assertSSHIngressIsPinnedTo(cidr string) {
	e.t.Helper()

	if admitted := e.sshIngressCIDRs(); len(admitted) != 1 || admitted[0] != cidr {
		e.t.Fatalf("the %s security group admits SSH from %v, want exactly [%s]", securityGroupName, admitted, cidr)
	}
}

func (e *ec2Environment) sshIngressCIDRs() []string {
	e.t.Helper()

	described, err := e.ec2API().DescribeSecurityGroups(context.Background(), &awsec2.DescribeSecurityGroupsInput{
		Filters: append(managedByIsolarium(), awsec2types.Filter{
			Name:   aws.String("group-name"),
			Values: []string{securityGroupName},
		}),
	})
	if err != nil {
		e.t.Fatalf("describing the %s security group: %v", securityGroupName, err)
	}

	var admitted []string
	for _, group := range described.SecurityGroups {
		for _, permission := range group.IpPermissions {
			if aws.ToInt32(permission.FromPort) != sshPort {
				continue
			}
			for _, ipRange := range permission.IpRanges {
				admitted = append(admitted, aws.ToString(ipRange.CidrIp))
			}
		}
	}
	return admitted
}

func (e *ec2Environment) assertPersistedIngressCIDRIs(cidr string) {
	e.t.Helper()

	persisted, err := ec2.ReadPersistedIngressCIDR(e.base)
	if err != nil {
		e.t.Fatalf("reading the recorded ingress CIDR: %v", err)
	}
	if persisted != cidr {
		e.t.Errorf("%s records ingress_cidr %q, want %q", ec2.TfvarsPath(e.base), persisted, cidr)
	}
}

// runEchoOK is `isolarium --name <name> --type ec2 run -- echo ok`, the
// observable's command, with the two output streams kept apart because the
// notice is expected on stderr and the command's own word on stdout.
func (e *ec2Environment) runEchoOK(environment processEnvironment) isolariumStreams {
	e.t.Helper()

	args := e.runArgs(false, "echo", i2codeEchoedWord)
	run := exec.Command(isolariumBinary(e.t), args...)
	run.Dir = e.workDir
	run.Env = environment

	var stdout, stderr bytes.Buffer
	run.Stdout = io.MultiWriter(&stdout, os.Stderr)
	run.Stderr = io.MultiWriter(&stderr, os.Stderr)

	exitCode, err := binaryExitCode(run.Run())
	if err != nil {
		e.t.Fatalf("isolarium %s: %v\n%s%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return isolariumStreams{t: e.t, exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

// isolariumStreams is what one invocation of the built binary reported, with
// stdout and stderr kept apart.
type isolariumStreams struct {
	t        *testing.T
	exitCode int
	stdout   string
	stderr   string
}

func (s isolariumStreams) assertPrintedOKAndExitedZero() {
	s.t.Helper()

	if s.exitCode != 0 {
		s.t.Errorf("isolarium run -- echo %s exited %d, want 0\n%s", i2codeEchoedWord, s.exitCode, s.stderr)
	}
	if !printsTheWord(s.stdout, i2codeEchoedWord) {
		s.t.Errorf("isolarium run -- echo %s did not print %q on stdout:\n%s", i2codeEchoedWord, i2codeEchoedWord, visibleText(s.stdout))
	}
}

func (s isolariumStreams) assertAnnouncedTheAddressChange(from, to string) {
	s.t.Helper()

	notice := fmt.Sprintf("%s %s to %s; updating SSH ingress...", hostAddressChangedNotice, from, to)
	if !strings.Contains(s.stderr, notice) {
		s.t.Errorf("the run against the locked-out group did not print %q on stderr:\n%s", notice, s.stderr)
	}
}

func (s isolariumStreams) assertAnnouncedNoAddressChange() {
	s.t.Helper()

	if strings.Contains(s.stderr, hostAddressChangedNotice) {
		s.t.Errorf("the second run announced an address change although the rule already held the host's address:\n%s", s.stderr)
	}
}

// refuseTerraform puts a terraform ahead of the real one on the binary's PATH
// that records being called and fails, so a run that made any terraform call
// at all would both leave a record and exit non-zero. "Makes no terraform call"
// is then observed rather than inferred from a quiet stderr.
func refuseTerraform(t *testing.T) (environment processEnvironment, invocations string) {
	t.Helper()

	dir := t.TempDir()
	invocations = filepath.Join(dir, "terraform-invocations")
	script := "#!/bin/sh\necho \"$@\" >> '" + invocations + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, terraformBinaryName), []byte(script), refusingTerraformMode); err != nil {
		t.Fatalf("writing the refusing terraform: %v", err)
	}
	path := dir + string(os.PathListSeparator) + os.Getenv("PATH")
	return append(binaryEnvironment(), "PATH="+path), invocations
}

const terraformBinaryName = "terraform"

func assertTerraformWasNotCalled(t *testing.T, invocations string) {
	t.Helper()

	if recorded, err := os.ReadFile(invocations); err == nil {
		t.Errorf("the second run called terraform although the host address was unchanged:\n%s", recorded)
	}
}
