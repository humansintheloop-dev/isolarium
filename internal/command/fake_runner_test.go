package command

import (
	"strings"
	"testing"
)

func TestFakeRunner_Calls_RecordsEveryInvocationInOrder(t *testing.T) {
	runner := NewFakeRunner(t)
	runner.OnCommand("terraform", "init").Returns("")
	runner.OnCommand("terraform", "apply").Returns("")

	_, _ = runner.Run("terraform", "init")
	_, _ = runner.Run("terraform", "apply")

	calls := runner.Calls()
	if len(calls) != 2 {
		t.Fatalf("Calls() recorded %d invocations, want 2", len(calls))
	}
	if got := strings.Join(calls[0], " "); got != "terraform init" {
		t.Errorf("Calls()[0] = %q, want %q", got, "terraform init")
	}
	if got := strings.Join(calls[1], " "); got != "terraform apply" {
		t.Errorf("Calls()[1] = %q, want %q", got, "terraform apply")
	}
}

// A catch-all registration on the command name is how most tests stub a whole
// tool, so one specific subcommand must still be able to answer differently.
func TestFakeRunner_Run_PrefersTheMostSpecificRegistration(t *testing.T) {
	runner := NewFakeRunner(t)
	runner.OnCommand("terraform").Returns("catch-all")
	runner.OnCommand("terraform", "version", "-json").Returns("version")

	for range 20 {
		assertRunReturns(t, runner, []string{"terraform", "version", "-json"}, "version")
		assertRunReturns(t, runner, []string{"terraform", "-chdir=/tmp", "output", "-json"}, "catch-all")
		assertRunReturns(t, runner, []string{"terraform", "version"}, "catch-all")
	}
}

func assertRunReturns(t *testing.T, runner *FakeRunner, command []string, want string) {
	t.Helper()

	output, err := runner.Run(command[0], command[1:]...)
	if err != nil {
		t.Fatalf("Run(%s) returned error: %v", strings.Join(command, " "), err)
	}
	if string(output) != want {
		t.Errorf("Run(%s) = %q, want %q", strings.Join(command, " "), output, want)
	}
}

func TestFakeRunner_VerifyExecuted_FailsWhenNotCalled(t *testing.T) {
	fakeT := &testing.T{}
	runner := NewFakeRunner(fakeT)
	runner.OnCommand("security", "find-generic-password").Returns("creds")

	runner.VerifyExecuted()

	if !fakeT.Failed() {
		t.Error("expected VerifyExecuted to fail when command was not called")
	}
}
