package cli

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func ec2RunWithSpy(t *testing.T, args ...string) *backendSpy {
	t.Helper()
	stubMintGitHubToken(t)
	spy := &backendSpy{}
	runWithSpy(t, spy, append([]string{"run", "--type", "ec2"}, args...))
	return spy
}

func TestRunCommand_EC2CallsBackendExec(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "echo", "hello")

	if !spy.execCalled {
		t.Fatal("expected backend.Exec to be called")
	}
	if spy.execName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.execName)
	}
	assertArgsEqual(t, "exec args", spy.execArgs, []string{"echo", "hello"})
}

func TestRunCommand_EC2InteractiveCallsBackendExecInteractive(t *testing.T) {
	spy := ec2RunWithSpy(t, "-i", "--", "claude")

	if !spy.execInteractiveCalled {
		t.Fatal("expected backend.ExecInteractive to be called")
	}
	if spy.execCalled {
		t.Fatal("expected backend.Exec NOT to be called for an interactive ec2 run")
	}
	if spy.execInteractiveName != "isolarium-ec2" {
		t.Errorf("expected name 'isolarium-ec2', got '%s'", spy.execInteractiveName)
	}
}

func TestRunCommand_EC2InjectsRunEnvVarsFromPidYaml(t *testing.T) {
	stubLoadRunEnvVars(t, map[string]string{"PID_VAR": "pid_value"})
	spy := ec2RunWithSpy(t, "--", "printenv", "PID_VAR")

	if spy.execEnvVars["PID_VAR"] != "pid_value" {
		t.Errorf("expected PID_VAR='pid_value', got '%s'", spy.execEnvVars["PID_VAR"])
	}
}

func TestRunCommand_EC2InjectsMintedGitHubToken(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "gh", "auth", "status")

	if spy.execEnvVars["GH_TOKEN"] != "test-token" {
		t.Errorf("expected GH_TOKEN 'test-token', got '%s'", spy.execEnvVars["GH_TOKEN"])
	}
	if spy.execEnvVars["GIT_TOKEN"] != "test-token" {
		t.Errorf("expected GIT_TOKEN 'test-token', got '%s'", spy.execEnvVars["GIT_TOKEN"])
	}
}

func TestRunCommand_EC2RejectsCreateFlag(t *testing.T) {
	stubMintGitHubToken(t)
	spy := &backendSpy{state: "none"}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"run", "--type", "ec2", "--create", "--", "echo", "hello"})

	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error when --create is used with --type ec2")
	}
	expected := "--create is not supported with --type ec2; run isolarium create --type ec2 first"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error %q, got: %v", expected, err)
	}
	if spy.createCalled {
		t.Error("expected backend.Create NOT to be called")
	}
	if spy.execCalled {
		t.Error("expected backend.Exec NOT to be called")
	}
}

func TestRunCommand_EC2DoesNotCallCopyCredentials(t *testing.T) {
	spy := ec2RunWithSpy(t, "--", "echo", "hello")

	if spy.copyCredentialsCalled {
		t.Fatal("expected CopyCredentials NOT to be called for ec2 until credential copying lands")
	}
}
