package ec2

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// remoteRunnerSpy answers a scripted sequence of exit codes and records what was
// asked of the instance, so a readiness loop can be driven without an instance.
type remoteRunnerSpy struct {
	exitCodes []int
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

func TestWaitForCloudInit_AsksTheInstanceToWaitForProvisioningToFinish(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{0}}
	sleeper := &sleepSpy{}

	if err := WaitForCloudInit(t.TempDir(), testPublicDNS, runner.run, sleeper.sleep); err != nil {
		t.Fatalf("WaitForCloudInit returned %v, want nil", err)
	}

	want := []string{"cloud-init", "status", "--wait"}
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

	if err := WaitForCloudInit(t.TempDir(), testPublicDNS, runner.run, sleeper.sleep); err != nil {
		t.Fatalf("WaitForCloudInit returned %v, want nil", err)
	}

	if len(runner.commands) != 3 {
		t.Errorf("attempted %d times, want 3", len(runner.commands))
	}
	if want := []time.Duration{cloudInitPollInterval, cloudInitPollInterval}; !reflect.DeepEqual(sleeper.calls, want) {
		t.Errorf("slept %v, want %v", sleeper.calls, want)
	}
}

func TestWaitForCloudInit_GivesUpAfterFifteenMinutes(t *testing.T) {
	runner := &remoteRunnerSpy{exitCodes: []int{255}}
	sleeper := &sleepSpy{}

	err := WaitForCloudInit(t.TempDir(), testPublicDNS, runner.run, sleeper.sleep)

	if err == nil {
		t.Fatal("WaitForCloudInit returned nil, want a timeout error")
	}
	if want := "instance did not finish cloud-init within 15m0s"; err.Error() != want {
		t.Errorf("WaitForCloudInit error = %q, want %q", err, want)
	}
	if sleeper.total() != CloudInitTimeout {
		t.Errorf("waited a total of %s, want %s", sleeper.total(), CloudInitTimeout)
	}
}

func TestWaitForCloudInit_ReportsAFailureToLaunchSSHWithoutRetrying(t *testing.T) {
	launchFailure := errors.New("ssh: executable file not found in $PATH")
	runner := &remoteRunnerSpy{exitCodes: []int{1}, err: launchFailure}
	sleeper := &sleepSpy{}

	err := WaitForCloudInit(t.TempDir(), testPublicDNS, runner.run, sleeper.sleep)

	if !errors.Is(err, launchFailure) {
		t.Fatalf("WaitForCloudInit error = %v, want it to wrap %v", err, launchFailure)
	}
	if len(runner.commands) != 1 {
		t.Errorf("attempted %d times, want 1 — a command that cannot be launched never recovers", len(runner.commands))
	}
}
