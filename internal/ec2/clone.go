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

// succeeded distinguishes an instance that is not answering yet, which is worth
// retrying, from a transport that cannot be launched at all, which never
// recovers.
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

// readinessLoop retries a probe on a fixed interval until the instance answers
// it, or the budget runs out.
type readinessLoop struct {
	budget    time.Duration
	interval  time.Duration
	probe     RemoteCommand
	giveUpMsg string
}

func (l readinessLoop) wait(session InstanceSession, sleep SleepFunc) error {
	for waited := time.Duration(0); waited < l.budget; waited += l.interval {
		ready, err := session.succeeded(l.probe)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		sleep(l.interval)
	}
	return fmt.Errorf("%s within %s", l.giveUpMsg, l.budget)
}

// sshReadiness covers the window in which AWS calls the instance running but
// sshd is not listening yet, so every attempt fails to connect at all.
var sshReadiness = readinessLoop{
	budget:    SSHTimeout,
	interval:  5 * time.Second,
	probe:     RemoteCommand{Args: []string{"true"}},
	giveUpMsg: "instance did not become reachable over SSH",
}

// cloudInitReadiness leans on `cloud-init status --wait`, which blocks on the
// instance until first-boot provisioning finishes.
var cloudInitReadiness = readinessLoop{
	budget:    CloudInitTimeout,
	interval:  15 * time.Second,
	probe:     RemoteCommand{Args: []string{"cloud-init", "status", "--wait"}},
	giveUpMsg: "instance did not finish cloud-init",
}

func WaitForSSH(session InstanceSession, sleep SleepFunc) error {
	return sshReadiness.wait(session, sleep)
}

func WaitForCloudInit(session InstanceSession, sleep SleepFunc) error {
	return cloudInitReadiness.wait(session, sleep)
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
