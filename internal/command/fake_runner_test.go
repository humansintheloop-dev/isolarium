package command

import "testing"

func TestFakeRunner_VerifyExecuted_FailsWhenNotCalled(t *testing.T) {
	fakeT := &testing.T{}
	runner := NewFakeRunner(fakeT)
	runner.OnCommand("security", "find-generic-password").Returns("creds")

	runner.VerifyExecuted()

	if !fakeT.Failed() {
		t.Error("expected VerifyExecuted to fail when command was not called")
	}
}
