package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

func runEC2Command(t *testing.T, home string, args ...string) (*bytes.Buffer, error) {
	t.Helper()

	t.Setenv("HOME", home)
	out := &bytes.Buffer{}
	rootCmd := NewRootCmd()
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs(append([]string{"ec2", "--env-file", filepath.Join(t.TempDir(), "absent.env")}, args...))
	return out, rootCmd.Execute()
}

func writeInstanceFiles(t *testing.T, base string, names ...string) {
	t.Helper()

	for _, name := range names {
		if err := ec2.WriteInstanceFile(base, name, "#cloud-config\n"); err != nil {
			t.Fatalf("writing the instance file for %q: %v", name, err)
		}
	}
}

func TestEC2WipeCommand_RefusesWhileEnvironmentsExist(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".isolarium")
	writeInstanceFiles(t, base, "my-work", "other")

	_, err := runEC2Command(t, home, "wipe")

	if err == nil {
		t.Fatal("ec2 wipe returned nil error while environments exist")
	}
	for _, want := range []string{
		"my-work",
		"other",
		"isolarium destroy --type ec2 --name my-work",
		"isolarium destroy --type ec2 --name other",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ec2 wipe error = %q, want it to contain %q", err.Error(), want)
		}
	}
	if _, statErr := os.Stat(ec2.InstanceFilePath(base, "my-work")); statErr != nil {
		t.Errorf("stat %s: %v, want the refused wipe to leave it alone", ec2.InstanceFilePath(base, "my-work"), statErr)
	}
}
