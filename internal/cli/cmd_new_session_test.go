package cli

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

const newSessionRejection = "--new-session is only supported with --type ec2"

func TestEC2NewSessionRejectedForNonEC2Types(t *testing.T) {
	for _, envType := range []string{"container", "vm", "nono"} {
		for _, invocation := range []struct {
			name string
			args []string
		}{
			{name: "run", args: []string{"run", "--type", envType, "-i", "--new-session", "--", "bash"}},
			{name: "shell", args: []string{"shell", "--type", envType, "--new-session"}},
		} {
			t.Run(envType+"/"+invocation.name, func(t *testing.T) {
				spy := &backendSpy{}
				rootCmd := newRootCmdWithResolver(func(string) (backend.Backend, error) { return spy, nil })
				rootCmd.SetArgs(invocation.args)

				err := rootCmd.Execute()

				if err == nil {
					t.Fatalf("%v returned nil error, want it rejected", invocation.args)
				}
				if !strings.Contains(err.Error(), newSessionRejection) {
					t.Errorf("error = %q, want it to contain %q", err, newSessionRejection)
				}
				assertNothingRan(t, spy)
			})
		}
	}
}

func assertNothingRan(t *testing.T, spy *backendSpy) {
	t.Helper()

	for _, reached := range []bool{spy.execCalled, spy.execInteractiveCalled, spy.openShellCalled} {
		if reached {
			t.Error("the rejected invocation still reached the backend")
		}
	}
}

func TestEC2NewSessionResolvesAFreshSessionNameOnTheBackend(t *testing.T) {
	b := &backend.EC2Backend{}

	applyNewSession(b, true)

	if b.SessionNameFunc == nil {
		t.Fatal("--new-session left the backend resolving the default session name")
	}
}

func TestEC2NewSessionLeavesTheDefaultSessionAloneWhenNotRequested(t *testing.T) {
	b := &backend.EC2Backend{}

	applyNewSession(b, false)

	if b.SessionNameFunc != nil {
		t.Error("a run without --new-session installed a session-name resolver")
	}
}
