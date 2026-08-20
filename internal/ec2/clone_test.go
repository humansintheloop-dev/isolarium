package ec2

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// remoteRunnerSpy answers a scripted sequence of exit codes — and, where the
// probe's output is what the caller reads, a matching sequence of outputs — and
// records what was asked of the instance, so a readiness loop can be driven
// without an instance.
type remoteRunnerSpy struct {
	exitCodes []int
	outputs   []string
	err       error
	commands  []RemoteCommand
}

func (s *remoteRunnerSpy) run(base, publicDNS string, cmd RemoteCommand) (int, error) {
	s.commands = append(s.commands, cmd)
	if s.err != nil {
		return 1, s.err
	}
	if len(s.commands) > len(s.exitCodes) {
		return s.exitCodes[len(s.exitCodes)-1], nil
	}
	return s.exitCodes[len(s.commands)-1], nil
}

func (s *remoteRunnerSpy) capture(base, publicDNS string, cmd RemoteCommand) (string, int, error) {
	exitCode, err := s.run(base, publicDNS, cmd)
	return s.scriptedOutput(len(s.commands) - 1), exitCode, err
}

func (s *remoteRunnerSpy) scriptedOutput(attempt int) string {
	if attempt >= len(s.outputs) {
		return ""
	}
	return s.outputs[attempt]
}

func (s *remoteRunnerSpy) session(t *testing.T) InstanceSession {
	t.Helper()

	return NewInstanceSession(t.TempDir(), testPublicDNS, s.run)
}

func (s *remoteRunnerSpy) query(t *testing.T) InstanceQuery {
	t.Helper()

	return NewInstanceQuery(t.TempDir(), testPublicDNS, s.capture)
}

// sleepSpy accumulates what the loop would have waited for, so a fifteen-minute
// budget is asserted without spending it.
type sleepSpy struct {
	calls []time.Duration
}

func (s *sleepSpy) sleep(d time.Duration) {
	s.calls = append(s.calls, d)
}

func (s *sleepSpy) total() time.Duration {
	var total time.Duration
	for _, call := range s.calls {
		total += call
	}
	return total
}

// readinessCase is one of the two waits create performs, so the give-up and
// launch-failure behaviour they share is asserted once for both.
type readinessCase struct {
	name      string
	wait      func(InstanceQuery, SleepFunc) error
	budget    time.Duration
	giveUpErr string
}

func readinessCases() []readinessCase {
	return []readinessCase{
		{"SSH", WaitForSSH, SSHTimeout, "instance did not become reachable over SSH within 5m0s"},
		{"cloud-init", WaitForCloudInit, CloudInitTimeout, "instance did not finish cloud-init within 15m0s"},
	}
}

func TestReadinessLoops_GiveUpOnceTheirBudgetIsSpent(t *testing.T) {
	for _, readiness := range readinessCases() {
		t.Run(readiness.name, func(t *testing.T) {
			runner := &remoteRunnerSpy{exitCodes: []int{255}}
			sleeper := &sleepSpy{}

			err := readiness.wait(runner.query(t), sleeper.sleep)

			if err == nil {
				t.Fatalf("waiting for %s returned nil, want a timeout error", readiness.name)
			}
			if err.Error() != readiness.giveUpErr {
				t.Errorf("error = %q, want %q", err, readiness.giveUpErr)
			}
			if sleeper.total() != readiness.budget {
				t.Errorf("waited a total of %s, want %s", sleeper.total(), readiness.budget)
			}
		})
	}
}

func TestReadinessLoops_ReportAFailureToLaunchSSHWithoutRetrying(t *testing.T) {
	for _, readiness := range readinessCases() {
		t.Run(readiness.name, func(t *testing.T) {
			launchFailure := errors.New("ssh: executable file not found in $PATH")
			runner := &remoteRunnerSpy{exitCodes: []int{1}, err: launchFailure}

			err := readiness.wait(runner.query(t), (&sleepSpy{}).sleep)

			if !errors.Is(err, launchFailure) {
				t.Fatalf("error = %v, want it to wrap %v", err, launchFailure)
			}
			if len(runner.commands) != 1 {
				t.Errorf("probed %d times, want 1 — a command that cannot be launched never recovers", len(runner.commands))
			}
		})
	}
}

func TestWaitForSSH_ProbesTheInstanceUntilItAnswers(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{255, 255, 0}}
	sleeper := &sleepSpy{}

	if err := WaitForSSH(runner.query(t), sleeper.sleep); err != nil {
		t.Fatalf("WaitForSSH returned %v, want nil", err)
	}

	if len(runner.commands) != 3 {
		t.Fatalf("probed %d times, want 3", len(runner.commands))
	}
	if want := []string{"true"}; !reflect.DeepEqual(runner.commands[0].Args, want) {
		t.Errorf("probe = %v, want %v", runner.commands[0].Args, want)
	}
	if want := []time.Duration{sshReadiness.interval, sshReadiness.interval}; !reflect.DeepEqual(sleeper.calls, want) {
		t.Errorf("slept %v, want %v", sleeper.calls, want)
	}
}

// TestWaitForSSH_KeepsRetryingWhateverTheProbeExitsWith pins the difference
// between the two loops: before the instance answers at all, no exit code
// distinguishes a failure worth reporting from one worth waiting out.
func TestWaitForSSH_KeepsRetryingWhateverTheProbeExitsWith(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{1, 2, 0}}
	sleeper := &sleepSpy{}

	if err := WaitForSSH(runner.query(t), sleeper.sleep); err != nil {
		t.Fatalf("WaitForSSH returned %v, want nil", err)
	}

	if len(runner.commands) != 3 {
		t.Errorf("probed %d times, want 3 — no exit code is terminal before the instance answers", len(runner.commands))
	}
}

func TestWaitForCloudInit_AsksTheInstanceToWaitForProvisioningToFinish(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{0}}
	sleeper := &sleepSpy{}

	if err := WaitForCloudInit(runner.query(t), sleeper.sleep); err != nil {
		t.Fatalf("WaitForCloudInit returned %v, want nil", err)
	}

	want := []string{"cloud-init", "status", "--wait", "--long"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].Args, want) {
		t.Errorf("remote commands = %v, want exactly one %v", runner.commands, want)
	}
	if len(sleeper.calls) != 0 {
		t.Errorf("slept %v, want no wait when the first attempt succeeds", sleeper.calls)
	}
}

func TestWaitForCloudInit_RetriesWhileTheInstanceIsStillUnreachable(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{255, 255, 0}}
	sleeper := &sleepSpy{}

	if err := WaitForCloudInit(runner.query(t), sleeper.sleep); err != nil {
		t.Fatalf("WaitForCloudInit returned %v, want nil", err)
	}

	if len(runner.commands) != 3 {
		t.Errorf("attempted %d times, want 3", len(runner.commands))
	}
	want := []time.Duration{cloudInitReadiness.interval, cloudInitReadiness.interval}
	if !reflect.DeepEqual(sleeper.calls, want) {
		t.Errorf("slept %v, want %v", sleeper.calls, want)
	}
}

// degradedCloudInitStatus is what `cloud-init status --wait --long` prints when
// provisioning ran to the end but a module failed on the way: the run is over —
// `status: done` — so re-probing it can only spend the budget.
const degradedCloudInitStatus = `.....
status: degraded done
extended_status: degraded done
last_update: Thu, 20 Aug 2026 14:03:21 +0000
detail: DataSourceEc2Local
errors: []
recoverable_errors:
	WARNING: Failed to run module install_claude_code
`

func TestWaitForCloudInit_FailsFastWhenProvisioningFinishesDegraded(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{2}, outputs: []string{degradedCloudInitStatus}}
	sleeper := &sleepSpy{}

	err := WaitForCloudInit(runner.query(t), sleeper.sleep)

	if err == nil {
		t.Fatal("WaitForCloudInit returned nil, want an error for a degraded provisioning run")
	}
	assertSettledWithoutRetrying(t, runner, sleeper)
	for _, want := range []string{"degraded", "install_claude_code", "/var/log/cloud-init-output.log"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "did not finish") {
		t.Errorf("error = %q, want it not to claim cloud-init never finished", err)
	}
}

func TestWaitForCloudInit_FailsFastWhenProvisioningErrors(t *testing.T) {
	errorStatus := "status: error\nerrors:\n\t- Failed to run module write_files\n"
	runner := &remoteRunnerSpy{exitCodes: []int{1}, outputs: []string{errorStatus}}
	sleeper := &sleepSpy{}

	err := WaitForCloudInit(runner.query(t), sleeper.sleep)

	if err == nil {
		t.Fatal("WaitForCloudInit returned nil, want an error for a failed provisioning run")
	}
	assertSettledWithoutRetrying(t, runner, sleeper)
	for _, want := range []string{"write_files", "/var/log/cloud-init-output.log"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

// assertSettledWithoutRetrying checks that the loop accepted the instance's
// answer as final, rather than spending the fifteen-minute budget on a run that
// is already over.
func assertSettledWithoutRetrying(t *testing.T, runner *remoteRunnerSpy, sleeper *sleepSpy) {
	t.Helper()

	if len(runner.commands) != 1 {
		t.Errorf("probed %d times, want 1 — the instance already reported the run over", len(runner.commands))
	}
	if len(sleeper.calls) != 0 {
		t.Errorf("slept %v, want no wait at all", sleeper.calls)
	}
}

const testCloneToken = "ghs_exampleclonetoken"

func testRepositorySpec(hostDir string) RepositorySpec {
	return RepositorySpec{
		Owner:       "humansintheloop-dev",
		Repo:        "isolarium",
		Branch:      "idea/ec2-isolation-type",
		Token:       testCloneToken,
		HostDir:     hostDir,
		AuthorEmail: "chris@example.com",
		AuthorName:  "Chris Richardson",
	}
}

func TestCloneRepo_ClonesTheRequestedBranchWithTheTokenOnlyInTheURL(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	err := CloneRepo(runner.session(t), testRepositorySpec(t.TempDir()))

	if err != nil {
		t.Fatalf("CloneRepo returned %v, want nil", err)
	}
	want := []RemoteCommand{
		{Args: []string{
			"git", "clone", "--branch", "idea/ec2-isolation-type",
			"https://x-access-token:" + testCloneToken + "@github.com/humansintheloop-dev/isolarium.git",
			"repo",
		}},
		{Workdir: RemoteRepoDir, Args: []string{
			"git", "remote", "set-url", "origin",
			"https://github.com/humansintheloop-dev/isolarium.git",
		}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Errorf("ran %v, want %v", runner.commands, want)
	}
}

// TestCloneRepo_LeavesNoTokenInTheRemoteGitRecords covers what git itself
// writes: the clone URL lands in .git/config, so the token has to be overwritten
// there rather than merely kept out of every command isolarium issues.
func TestCloneRepo_LeavesNoTokenInTheRemoteGitRecords(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	if err := CloneRepo(runner.session(t), testRepositorySpec(t.TempDir())); err != nil {
		t.Fatalf("CloneRepo returned %v, want nil", err)
	}

	last := runner.commands[len(runner.commands)-1]
	if strings.Contains(strings.Join(last.Args, " "), testCloneToken) {
		t.Errorf("the last command %v still carries the token, so .git/config keeps it", last.Args)
	}
}

func TestCloneRepo_ReportsANonZeroExitWithoutRepeatingTheToken(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{128}}

	err := CloneRepo(runner.session(t), testRepositorySpec(t.TempDir()))

	if err == nil {
		t.Fatal("CloneRepo returned nil, want an error for a failed clone")
	}
	if strings.Contains(err.Error(), testCloneToken) {
		t.Errorf("CloneRepo error = %q, want it not to repeat the clone token", err)
	}
}

func TestConfigureGitAuthor_IsolatesTheEmailAndSuffixesTheName(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	err := ConfigureGitAuthor(runner.session(t), testRepositorySpec(t.TempDir()))

	if err != nil {
		t.Fatalf("ConfigureGitAuthor returned %v, want nil", err)
	}
	want := []RemoteCommand{
		{Workdir: RemoteRepoDir, Args: []string{"git", "config", "user.email", "'chris+i2code@example.com'"}},
		{Workdir: RemoteRepoDir, Args: []string{"git", "config", "user.name", "'Chris Richardson - i2code'"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Errorf("ran %v, want %v", runner.commands, want)
	}
}

func TestCopyFileToInstance_WritesTheHostContentsUnderTheirParentDirectory(t *testing.T) {
	hostDir := t.TempDir()
	localPath := filepath.Join(hostDir, "CLAUDE.md")
	contents := "# Project Guidelines\n'quoted' & $dangerous\n"
	writeHostFile(t, localPath, contents)
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	err := CopyFileToInstance(runner.session(t), localPath, RemoteRepoDir+"/CLAUDE.md")

	if err != nil {
		t.Fatalf("CopyFileToInstance returned %v, want nil", err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(contents))
	want := []string{"bash", "-c", "'mkdir -p " + RemoteRepoDir +
		" && echo " + encoded + " | base64 -d > " + RemoteRepoDir + "/CLAUDE.md'"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].Args, want) {
		t.Errorf("ran %v, want exactly one %v", runner.commands, want)
	}
}

func TestCopyFileToInstance_ReportsAnUnreadableHostFile(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	err := CopyFileToInstance(runner.session(t), filepath.Join(t.TempDir(), "absent.md"), RemoteRepoDir+"/absent.md")

	if err == nil {
		t.Fatal("CopyFileToInstance returned nil, want an error for a file that is not on the host")
	}
	if len(runner.commands) != 0 {
		t.Errorf("ran %v, want nothing on the instance", runner.commands)
	}
}

func writeHostFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestPlaceRepository_ClonesConfiguresTheAuthorAndCopiesTheProjectConfig(t *testing.T) {
	hostDir := t.TempDir()
	writeHostFile(t, filepath.Join(hostDir, ".claude", "settings.local.json"), `{"permissions":{}}`)
	writeHostFile(t, filepath.Join(hostDir, "CLAUDE.md"), "# Project Guidelines\n")
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	if err := PlaceRepository(runner.session(t), testRepositorySpec(hostDir)); err != nil {
		t.Fatalf("PlaceRepository returned %v, want nil", err)
	}

	assertRemoteCommandOrder(t, runner.commands, []string{
		"git clone",
		"git remote set-url origin",
		"git config user.email",
		"git config user.name",
		RemoteRepoDir + "/.claude/settings.local.json",
		RemoteRepoDir + "/CLAUDE.md",
	})
}

func TestPlaceRepository_SkipsProjectConfigTheHostDoesNotHave(t *testing.T) {
	hostDir := t.TempDir()
	writeHostFile(t, filepath.Join(hostDir, "CLAUDE.md"), "# Project Guidelines\n")
	runner := &remoteRunnerSpy{exitCodes: []int{0}}

	if err := PlaceRepository(runner.session(t), testRepositorySpec(hostDir)); err != nil {
		t.Fatalf("PlaceRepository returned %v, want nil", err)
	}

	assertRemoteCommandOrder(t, runner.commands, []string{
		"git clone",
		"git remote set-url origin",
		"git config user.email",
		"git config user.name",
		RemoteRepoDir + "/CLAUDE.md",
	})
}

// assertRemoteCommandOrder checks that the recorded commands are exactly as many
// as expected and that each contains its marker, so the sequence is pinned
// without restating every argument.
func assertRemoteCommandOrder(t *testing.T, commands []RemoteCommand, markers []string) {
	t.Helper()

	if len(commands) != len(markers) {
		t.Fatalf("ran %d commands, want %d: %v", len(commands), len(markers), commands)
	}
	for i, marker := range markers {
		if got := strings.Join(commands[i].Args, " "); !strings.Contains(got, marker) {
			t.Errorf("command %d = %q, want it to contain %q", i, got, marker)
		}
	}
}
