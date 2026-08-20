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

func TestFakeRunner_VerifyExecuted_FailsWhenNotCalled(t *testing.T) {
	fakeT := &testing.T{}
	runner := NewFakeRunner(fakeT)
	runner.OnCommand("security", "find-generic-password").Returns("creds")

	runner.VerifyExecuted()

	if !fakeT.Failed() {
		t.Error("expected VerifyExecuted to fail when command was not called")
	}
}
