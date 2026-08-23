package ec2

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/humansintheloop-dev/isolarium/internal/git"
)

// SSHTimeout caps how long create waits for a freshly applied instance to start
// answering on port 22.
const SSHTimeout = 5 * time.Minute

// CloudInitTimeout caps how long create waits for first-boot provisioning. An
// instance that has not finished the toolchain by then is not going to.
const CloudInitTimeout = 15 * time.Minute

// cloudInitLogPath is where the instance keeps everything first-boot
// provisioning printed, which is the only place a failed module's own output
// survives.
const cloudInitLogPath = "/var/log/cloud-init-output.log"

// sshTransportFailureExit is the code ssh reserves for its own failures. On a
// freshly launched instance it means sshd is not listening yet, which is the one
// probe result worth waiting out.
const sshTransportFailureExit = 255

// cloudInitDegradedExit is what `cloud-init status` reports when provisioning
// ran to the end with warnings logged and no module failed.
const cloudInitDegradedExit = 2

// isolationNameSuffix distinguishes commits authored inside an isolated
// environment from ones authored on the host, matching the Lima flow.
const isolationNameSuffix = " - i2code"

// projectConfigFiles travel from the host checkout into the instance, because
// they are deliberately untracked and so do not arrive with the clone.
var projectConfigFiles = []string{
	".claude/settings.local.json",
	"CLAUDE.md",
}

// RemoteRunner runs cmd on the instance reachable at publicDNS and returns the
// remote command's exit code.
type RemoteRunner func(base, publicDNS string, cmd RemoteCommand) (int, error)

// SleepFunc is the pause between attempts of a readiness loop, injected so tests
// can spend the budget instantly.
type SleepFunc func(time.Duration)

// InstanceSession is the ability to run commands on one instance: where its SSH
// material lives, where it can be reached, and the transport that carries the
// command there.
type InstanceSession struct {
	base      string
	publicDNS string
	run       RemoteRunner
}

func NewInstanceSession(base, publicDNS string, run RemoteRunner) InstanceSession {
	return InstanceSession{base: base, publicDNS: publicDNS, run: run}
}

// exitCode runs cmd on the instance and reports the remote command's exit
// status.
func (s InstanceSession) exitCode(cmd RemoteCommand) (int, error) {
	return s.run(s.base, s.publicDNS, cmd)
}

// succeeded reports whether the instance accepted the command, keeping a
// transport that could not be launched at all separate from one the instance
// itself rejected.
func (s InstanceSession) succeeded(cmd RemoteCommand) (bool, error) {
	exitCode, err := s.exitCode(cmd)
	if err != nil {
		return false, err
	}
	return exitCode == 0, nil
}

// mustRun turns a non-zero remote exit code into an error, because a
// provisioning step has no exit status worth propagating — it either happened or
// create cannot continue.
func (s InstanceSession) mustRun(cmd RemoteCommand, description string) error {
	succeeded, err := s.succeeded(cmd)
	if err != nil {
		return err
	}
	if !succeeded {
		return fmt.Errorf("failed to %s on the instance", description)
	}
	return nil
}

// probeOutcome is what a readiness loop should do about one probe result: stop
// because the instance is ready, stop because no amount of waiting can change
// the answer, or wait and probe again.
type probeOutcome struct {
	settled bool
	// notice is what a ready instance said that is worth relaying to whoever
	// asked — nothing, for a clean run.
	notice string
	err    error
}

func instanceIsReady() probeOutcome { return probeOutcome{settled: true} }

func instanceIsReadyAndSays(notice string) probeOutcome {
	return probeOutcome{settled: true, notice: notice}
}

func waitingCannotHelp(err error) probeOutcome { return probeOutcome{settled: true, err: err} }

func notReadyYet() probeOutcome { return probeOutcome{} }

// probeResult is one answer the instance gave a probe: what the command exited
// with and what it printed on the way.
type probeResult struct {
	exitCode int
	output   string
}

// readinessLoop retries a probe on a fixed interval until its outcome settles,
// or the budget runs out. What each exit code means belongs to the probe rather
// than to the loop, so assess decides and the loop only carries out the verdict.
type readinessLoop struct {
	budget    time.Duration
	interval  time.Duration
	probe     RemoteCommand
	assess    func(probeResult) probeOutcome
	giveUpMsg string
}

// wait hands back the notice of the outcome that settled the loop alongside its
// error, so a probe that found the instance ready with something to mention can
// pass that on.
func (l readinessLoop) wait(query InstanceQuery, sleep SleepFunc) (string, error) {
	for waited := time.Duration(0); waited < l.budget; waited += l.interval {
		output, exitCode, err := query.capture(l.probe)
		if err != nil {
			return "", err
		}
		if outcome := l.assess(probeResult{exitCode: exitCode, output: output}); outcome.settled {
			return outcome.notice, outcome.err
		}
		sleep(l.interval)
	}
	return "", fmt.Errorf("%s within %s", l.giveUpMsg, l.budget)
}

// everyFailureMeansNotUpYet is how the SSH probe reads its exit codes: until the
// instance answers at all, nothing distinguishes a failure worth reporting from
// one worth waiting out.
func everyFailureMeansNotUpYet(result probeResult) probeOutcome {
	if result.exitCode == 0 {
		return instanceIsReady()
	}
	return notReadyYet()
}

// assessCloudInit separates the answers the instance itself gives — provisioning
// finished, finished degraded, or failed outright — from the ssh transport
// failure that means sshd is not listening yet. Only that last one is worth
// probing again for; the other three are the instance's final word.
//
// Degraded is cloud-init saying every module ran and something logged a
// warning on the way — a module that fails is recorded under `errors` and
// reported as `status: error` instead. The Ubuntu AMI's IMDS probe over IPv6,
// on an instance that has no IPv6, is enough to make every run degraded, so the
// instance is taken as ready and the warnings are relayed rather than acted on.
func assessCloudInit(result probeResult) probeOutcome {
	report := cloudInitReport(result.output)
	switch result.exitCode {
	case 0:
		return instanceIsReady()
	case sshTransportFailureExit:
		return notReadyYet()
	case cloudInitDegradedExit:
		return instanceIsReadyAndSays(report.warnings())
	default:
		return waitingCannotHelp(report.failure())
	}
}

// cloudInitReport is what `cloud-init status --wait --long` printed: the
// instance's own account of how first-boot provisioning went.
type cloudInitReport string

// failure reports a provisioning run the instance has declared over with a
// module failed, quoting the status it gave so the failing modules are named,
// and pointing at the log that holds the rest.
func (r cloudInitReport) failure() error {
	return fmt.Errorf("cloud-init failed on the instance:\n%s\nsee %s on the instance for the full output",
		r.summary(), cloudInitLogPath)
}

// warnings is the notice for a degraded run: the warnings cloud-init recorded,
// quoted in full, and where the rest of the output is.
func (r cloudInitReport) warnings() string {
	return fmt.Sprintf("Warning: cloud-init finished degraded on the instance — it logged warnings, but no module failed:\n%s\nsee %s on the instance for the full output",
		r.summary(), cloudInitLogPath)
}

// summary is what the long status said, without the progress dots `--wait`
// prints while first-boot provisioning is still running.
func (r cloudInitReport) summary() string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(string(r)), "."))
}

// sshReadiness covers the window in which AWS calls the instance running but
// sshd is not listening yet, so every attempt fails to connect at all.
var sshReadiness = readinessLoop{
	budget:    SSHTimeout,
	interval:  5 * time.Second,
	probe:     RemoteCommand{Args: []string{"true"}},
	assess:    everyFailureMeansNotUpYet,
	giveUpMsg: "instance did not become reachable over SSH",
}

// cloudInitReadiness leans on `cloud-init status --wait`, which blocks on the
// instance until first-boot provisioning finishes, and asks for the long report
// so a run that finished degraded can name the modules that failed.
var cloudInitReadiness = readinessLoop{
	budget:    CloudInitTimeout,
	interval:  15 * time.Second,
	probe:     RemoteCommand{Args: []string{"cloud-init", "status", "--wait", "--long"}},
	assess:    assessCloudInit,
	giveUpMsg: "instance did not finish cloud-init",
}

func WaitForSSH(query InstanceQuery, sleep SleepFunc) error {
	_, err := sshReadiness.wait(query, sleep)
	return err
}

// WaitForCloudInit returns once provisioning is over. The notice is empty for a
// clean run and carries cloud-init's own warnings for a degraded one, so the
// caller can relay them; an error means the instance cannot be worked in.
func WaitForCloudInit(query InstanceQuery, sleep SleepFunc) (string, error) {
	return cloudInitReadiness.wait(query, sleep)
}

// RepositorySpec is everything the instance needs in order to end up holding the
// repository: where to clone it from, at which branch, who to attribute commits
// to, and which host checkout the project config travels from.
type RepositorySpec struct {
	Owner       string
	Repo        string
	Branch      string
	Token       string
	HostDir     string
	AuthorEmail string
	AuthorName  string
}

// cloneURL embeds the installation token in the URL of the single command that
// needs it, so the token never lands on the instance's disk.
func (s RepositorySpec) cloneURL() string {
	return "https://x-access-token:" + s.Token + "@github.com/" + s.Owner + "/" + s.Repo + ".git"
}

// remoteURL is the credential-free origin the clone is left pointing at. Each
// run injects a fresh token instead, so the instance never holds one that
// outlives the command it was minted for.
func (s RepositorySpec) remoteURL() string {
	return "https://github.com/" + s.Owner + "/" + s.Repo + ".git"
}

// isolatedAuthor is who commits made inside the instance are attributed to, kept
// distinguishable from the same person's host commits.
func (s RepositorySpec) isolatedAuthor() (email, name string) {
	return git.TransformEmailForIsolation(s.AuthorEmail), s.AuthorName + isolationNameSuffix
}

// PlaceRepository puts the repository inside the instance at RemoteRepoDir,
// attributes commits made there to the isolated author, and carries the host's
// project config across.
func PlaceRepository(session InstanceSession, spec RepositorySpec) error {
	if err := CloneRepo(session, spec); err != nil {
		return err
	}
	if err := ConfigureGitAuthor(session, spec); err != nil {
		return err
	}
	return copyProjectConfig(session, spec.HostDir)
}

func CloneRepo(session InstanceSession, spec RepositorySpec) error {
	clone := RemoteCommand{Args: []string{"git", "clone", "--branch", spec.Branch, spec.cloneURL(), "repo"}}
	if err := session.mustRun(clone, "clone the repository"); err != nil {
		return err
	}
	return stripTokenFromOrigin(session, spec)
}

// stripTokenFromOrigin overwrites the remote URL git records in .git/config,
// which is the one place the clone token would otherwise survive the command
// that carried it.
func stripTokenFromOrigin(session InstanceSession, spec RepositorySpec) error {
	setURL := RemoteCommand{
		Workdir: RemoteRepoDir,
		Args:    []string{"git", "remote", "set-url", "origin", spec.remoteURL()},
	}
	return session.mustRun(setURL, "strip the token from the origin URL")
}

func ConfigureGitAuthor(session InstanceSession, spec RepositorySpec) error {
	email, name := spec.isolatedAuthor()
	for _, setting := range []struct{ key, value string }{{"user.email", email}, {"user.name", name}} {
		cmd := RemoteCommand{
			Workdir: RemoteRepoDir,
			Args:    []string{"git", "config", setting.key, shellQuote(setting.value)},
		}
		if err := session.mustRun(cmd, "configure git "+setting.key); err != nil {
			return err
		}
	}
	return nil
}

// copyProjectConfig carries across only the files the host actually has, so a
// checkout without them is not a failure.
func copyProjectConfig(session InstanceSession, hostDir string) error {
	for _, name := range projectConfigFiles {
		localPath := filepath.Join(hostDir, name)
		if !fileExists(localPath) {
			continue
		}
		if err := CopyFileToInstance(session, localPath, RemoteRepoDir+"/"+name); err != nil {
			return err
		}
	}
	return nil
}

// CopyFileToInstance writes the host file's contents to remotePath, creating its
// parent directory. The contents travel base64-encoded so that no quoting,
// newline, or shell metacharacter in them can alter the remote command.
func CopyFileToInstance(session InstanceSession, localPath, remotePath string) error {
	contents, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", localPath, err)
	}

	script := "mkdir -p " + remoteParentDir(remotePath) +
		" && echo " + base64.StdEncoding.EncodeToString(contents) +
		" | base64 -d > " + remotePath
	return session.mustRun(RemoteCommand{Args: []string{"bash", "-c", shellQuote(script)}}, "write "+remotePath)
}

func remoteParentDir(remotePath string) string {
	return remotePath[:strings.LastIndex(remotePath, "/")]
}

// shellQuote protects a value from the remote shell, which re-parses everything
// ssh sends it as a single command line.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
