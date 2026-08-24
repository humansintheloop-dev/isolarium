package backend

import (
	"strconv"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

// ec2RemoteLog is the order in which the backend's remote collaborators were
// asked to do things, across every transport, so a test can say what must have
// happened before what.
type ec2RemoteLog struct {
	lines []string
}

func (l *ec2RemoteLog) record(cmd ec2.RemoteCommand) {
	if l != nil {
		l.lines = append(l.lines, strings.Join(cmd.Args, " "))
	}
}

// ec2ExecSpy stands in for the SSH transport, recording what Exec asked it to
// run and returning a canned exit code.
type ec2ExecSpy struct {
	called    bool
	base      string
	publicDNS string
	command   ec2.RemoteCommand
	exitCode  int
	err       error
	log       *ec2RemoteLog
}

func (s *ec2ExecSpy) exec(base, publicDNS string, cmd ec2.RemoteCommand) (int, error) {
	s.called = true
	s.base = base
	s.publicDNS = publicDNS
	s.command = cmd
	s.log.record(cmd)
	return s.exitCode, s.err
}

// ec2InstanceFake answers the backend's reads of the instance over the capture
// transport — the command a running session recorded, the sessions running, and
// the exit status file — so a test can say what the instance holds and see what
// was asked of it, and where.
type ec2InstanceFake struct {
	t           *testing.T
	recorded    string // what the session's @isolarium-command option holds
	sessions    string // what `tmux list-sessions` prints
	status      string // what the status file holds; empty when it is absent
	clearExit   int    // how the instance answers the clear of the status file
	unreachable string // an address every read fails to connect to
	reads       []ec2InstanceRead
	log         *ec2RemoteLog
}

// ec2InstanceRead is one question asked of the instance, and the address it was
// asked at.
type ec2InstanceRead struct {
	publicDNS string
	line      string
}

func (f *ec2InstanceFake) capture(base, publicDNS string, cmd ec2.RemoteCommand) (string, int, error) {
	line := strings.Join(cmd.Args, " ")
	f.reads = append(f.reads, ec2InstanceRead{publicDNS: publicDNS, line: line})
	f.log.record(cmd)
	if publicDNS == f.unreachable {
		return "", 255, nil
	}
	switch {
	case strings.HasPrefix(line, "tmux show-option"):
		return f.recorded + "\n", 0, nil
	case strings.HasPrefix(line, "tmux list-sessions"):
		return f.sessions, 0, nil
	case strings.HasPrefix(line, "mkdir -p ~/.isolarium && rm -f ~/.isolarium/status-"):
		return "", f.clearExit, nil
	case strings.HasPrefix(line, "cat ~/.isolarium/status-"):
		return f.statusFileContents()
	}
	f.t.Errorf("the backend read something unexpected from the instance: %s", line)
	return "", 1, nil
}

func (f *ec2InstanceFake) statusFileContents() (string, int, error) {
	if f.status == "" {
		return "", 1, nil
	}
	return f.status + "\n", 0, nil
}

// asked reports whether any read began with prefix.
func (f *ec2InstanceFake) asked(prefix string) bool {
	return len(f.addressesAsked(prefix)) > 0
}

// addressesAsked lists where each read beginning with prefix was sent.
func (f *ec2InstanceFake) addressesAsked(prefix string) []string {
	var addresses []string
	for _, read := range f.reads {
		if strings.HasPrefix(read.line, prefix) {
			addresses = append(addresses, read.publicDNS)
		}
	}
	return addresses
}

// ec2ExecFixture is a backend whose environment "my-work" has already been
// created from the address the host still has, so Exec has metadata to read and
// no ingress rule to re-apply, with every remote collaborator spied on. The
// plain transport (exec) also answers the `tmux has-session` probe, so its exit
// code decides whether the instance is running a session. The session transport
// exits 0 as a tmux client does whatever its command did; the exit code a test
// asks for is what the instance's status file holds. The terraform fake answers
// nothing, so any invocation fails the test unless it registers one.
type ec2ExecFixture struct {
	backend     *EC2Backend
	exec        *ec2ExecSpy
	interactive *ec2ExecSpy
	session     *ec2ExecSpy
	instance    *ec2InstanceFake
	log         *ec2RemoteLog
	bucket      *ensureBucketSpy
	terraform   *command.FakeRunner
}

func ec2BackendWithRecordedInstance(t *testing.T, exitCode int) ec2ExecFixture {
	t.Helper()

	runner := command.NewFakeRunner(t)
	bucket := &ensureBucketSpy{name: ec2SpyBucket}
	log := &ec2RemoteLog{}
	execSpy := &ec2ExecSpy{exitCode: exitCode, log: log}
	interactiveSpy := &ec2ExecSpy{exitCode: exitCode, log: log}
	sessionSpy := &ec2ExecSpy{exitCode: 0, log: log}
	instance := &ec2InstanceFake{t: t, status: strconv.Itoa(exitCode), log: log}

	fixture := ec2BackendFixture{
		env:         map[string]string{"AWS_REGION": "us-west-2"},
		bucket:      bucket,
		host:        newHostProvisioningSpy(),
		remote:      &ec2RemoteSpy{},
		repository:  newRepositorySourceSpy(t),
		metadataDir: t.TempDir(),
		workDir:     t.TempDir(),
		runner:      runner,
	}
	seedRecordedInstance(t, fixture.metadataDir)
	persistIngressCIDR(t, fixture.metadataDir, ec2SpyDetectedCIDR)

	b := fixture.backend()
	b.ExecFunc = execSpy.exec
	b.ExecInteractiveFunc = interactiveSpy.exec
	b.ExecInSessionFunc = sessionSpy.exec
	b.CaptureFunc = instance.capture
	return ec2ExecFixture{
		backend:     b,
		exec:        execSpy,
		interactive: interactiveSpy,
		session:     sessionSpy,
		instance:    instance,
		log:         log,
		bucket:      bucket,
		terraform:   runner,
	}
}

// ec2BackendWithIdleInstance is a recorded instance on which no tmux session is
// running, so Exec starts one rather than finding one.
func ec2BackendWithIdleInstance(t *testing.T, exitCode int) ec2ExecFixture {
	t.Helper()

	f := ec2BackendWithRecordedInstance(t, exitCode)
	f.exec.exitCode = 1
	return f
}

func seedRecordedInstance(t *testing.T, metadataDir string) {
	t.Helper()

	err := ec2.NewMetadataStore(metadataDir, "my-work").Write(ec2.Metadata{
		InstanceID: ec2SpyInstanceID,
		PublicDNS:  ec2SpyPublicDNS,
		Region:     "us-west-2",
		CreatedAt:  ec2SpyCreatedAt,
	})
	if err != nil {
		t.Fatalf("seeding metadata: %v", err)
	}
}

func TestEC2Exec_BackendPropagatesExitCode(t *testing.T) {
	f := ec2BackendWithIdleInstance(t, 42)

	exitCode, err := f.backend.Exec(ExecRequest{
		ContainerName: "my-work",
		EnvVars:       map[string]string{"GH_TOKEN": "tok123"},
		Args:          []string{"sh", "-c", "exit 42"},
	})

	if err != nil {
		t.Fatalf("Exec() error = %v, want nil", err)
	}
	if exitCode != 42 {
		t.Errorf("Exec() exit code = %d, want 42", exitCode)
	}
	assertExecUsedRecordedInstance(t, f.session, f.backend.MetadataDir)
	assertNoRemoteInfrastructureCalls(t, f.bucket, f.terraform)
}

func assertExecUsedRecordedInstance(t *testing.T, spy *ec2ExecSpy, metadataDir string) {
	t.Helper()

	if !spy.called {
		t.Fatal("Exec() did not reach the session SSH transport")
	}
	if spy.publicDNS != ec2SpyPublicDNS {
		t.Errorf("Exec() used public DNS %q, want the one recorded in metadata.json (%q)", spy.publicDNS, ec2SpyPublicDNS)
	}
	if spy.base != metadataDir {
		t.Errorf("Exec() used base %q, want %q", spy.base, metadataDir)
	}
	if spy.command.EnvVars["GH_TOKEN"] != "tok123" {
		t.Errorf("Exec() passed GH_TOKEN %q, want %q", spy.command.EnvVars["GH_TOKEN"], "tok123")
	}
	assertArgsEqual(t, "exec args", spy.command.Args, sessionStart("isolarium", "sh -c 'exit 42'"))
}

func assertArgsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()

	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func assertNoRemoteInfrastructureCalls(t *testing.T, bucket *ensureBucketSpy, runner *command.FakeRunner) {
	t.Helper()

	if bucket.called {
		t.Error("Exec() made an AWS SDK call, want metadata.json to be the only source")
	}
	if len(runner.Calls()) != 0 {
		t.Errorf("Exec() ran %v, want no terraform invocation", runner.Calls())
	}
}

func TestEC2Exec_BackendFailsWhenTheEnvironmentWasNeverCreated(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 0)

	exitCode, err := f.backend.Exec(ExecRequest{ContainerName: "never-created", Args: []string{"echo", "hello"}})

	if err == nil {
		t.Fatal("Exec() returned nil error for an environment with no metadata.json")
	}
	if exitCode != 1 {
		t.Errorf("Exec() exit code = %d, want 1", exitCode)
	}
	if f.exec.called || f.session.called {
		t.Error("Exec() reached the SSH transport despite the missing metadata")
	}
}

// TestEC2Backend_Create_NeverUsesTheSessionTransport pins that the transports
// create uses internally — the readiness probes, the clone, the pid.yaml
// scripts — stay outside tmux and off the forced pseudo-terminal.
func TestEC2Backend_Create_NeverUsesTheSessionTransport(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	session := &ec2ExecSpy{}
	b := fixture.backend()
	b.ExecInSessionFunc = session.exec

	if err := b.Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if session.called {
		t.Errorf("Create() ran %v over the session transport, want it to stay on the plain one", session.command.Args)
	}
	if len(fixture.remote.commands) == 0 {
		t.Fatal("Create() ran nothing over the plain transport")
	}
	if fixture.remote.ranCommand("tmux") {
		t.Errorf("Create() wrapped a command in tmux: %v", fixture.remote.commands)
	}
}

func TestEC2ExecInteractive_BackendPropagatesExitCode(t *testing.T) {
	f := ec2BackendWithRecordedInstance(t, 7)

	exitCode, err := f.backend.ExecInteractive(ExecRequest{ContainerName: "my-work", Args: []string{"claude"}})

	if err != nil {
		t.Fatalf("ExecInteractive() error = %v, want nil", err)
	}
	if exitCode != 7 {
		t.Errorf("ExecInteractive() exit code = %d, want 7", exitCode)
	}
	if !f.interactive.called {
		t.Fatal("ExecInteractive() did not reach the interactive SSH transport")
	}
	assertSessionProbe(t, f.exec)
	if f.interactive.publicDNS != ec2SpyPublicDNS {
		t.Errorf("ExecInteractive() used public DNS %q, want %q", f.interactive.publicDNS, ec2SpyPublicDNS)
	}
}
