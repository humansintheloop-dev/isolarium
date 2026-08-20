package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func destroyEC2Command(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	out := &bytes.Buffer{}
	rootCmd := NewRootCmd()
	rootCmd.SetOut(out)
	rootCmd.SetArgs(append([]string{"destroy", "--env-file", filepath.Join(t.TempDir(), "absent.env")}, args...))
	return out, rootCmd.Execute()
}

func TestEC2DestroyCommand_ReportsNothingToDestroyForAnEnvironmentThatWasNeverCreated(t *testing.T) {
	out, err := destroyEC2Command(t, "--type", "ec2", "--name", "my-work")

	if err != nil {
		t.Fatalf("destroy --type ec2 error = %v, want nil", err)
	}
	if got := out.String(); !strings.Contains(got, "no EC2 environment to destroy") {
		t.Errorf("destroy --type ec2 printed %q, want it to contain %q", got, "no EC2 environment to destroy")
	}
}

func TestEC2DestroyCommand_UsesTheEC2DefaultName(t *testing.T) {
	out, err := destroyEC2Command(t, "--type", "ec2")

	if err != nil {
		t.Fatalf("destroy --type ec2 error = %v, want nil", err)
	}
	if got := out.String(); !strings.Contains(got, "no EC2 environment to destroy") {
		t.Errorf("destroy --type ec2 printed %q, want it to contain %q", got, "no EC2 environment to destroy")
	}
}
