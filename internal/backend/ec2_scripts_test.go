package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/ec2"
)

const (
	creationScriptPath = "scripts/creation.sh"
	envScriptPath      = "scripts/env.sh"
	hostScriptPath     = "scripts/host.sh"
	hostMarkerName     = "host-ran"
)

// pidYamlDeclaringOneScriptOfEachKind returns a work directory whose pid.yaml
// asks the ec2 backend for a creation script, a host script, and an env script,
// so one Create exercises all three hooks.
func pidYamlDeclaringOneScriptOfEachKind(t *testing.T) string {
	t.Helper()

	workDir := t.TempDir()
	writeHostProjectFile(t, filepath.Join(workDir, "pid.yaml"), `isolarium:
  ec2:
    create:
      creation_scripts:
        - path: `+creationScriptPath+`
      post_creation_scripts:
        host_scripts:
          - path: `+hostScriptPath+`
        env_scripts:
          - path: `+envScriptPath+`
`)
	writeHostMarkerScript(t, filepath.Join(workDir, hostScriptPath), filepath.Join(workDir, hostMarkerName))
	return workDir
}

// writeHostMarkerScript writes a host script that records the environment it was
// handed, so a script run with the wrong one cannot pass.
func writeHostMarkerScript(t *testing.T, path, markerPath string) {
	t.Helper()

	script := "#!/bin/sh\nprintf '%s %s' \"$ISOLARIUM_NAME\" \"$ISOLARIUM_TYPE\" > " + markerPath + "\n"
	writeHostProjectFile(t, path, script)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatalf("making %s executable: %v", path, err)
	}
}

func TestEC2Backend_Create_RunsThePidYamlScriptsInsideTheInstance(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	fixture.workDir = pidYamlDeclaringOneScriptOfEachKind(t)

	if err := fixture.backend().Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	scripts := lastRemoteCommands(t, fixture.remote.commands, 2)
	assertScriptRanInTheClone(t, scripts[0], creationScriptPath)
	assertScriptRanInTheClone(t, scripts[1], envScriptPath)
}

func TestEC2Backend_Create_RunsThePidYamlHostScriptOnTheHost(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	fixture.workDir = pidYamlDeclaringOneScriptOfEachKind(t)

	if err := fixture.backend().Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertHostMarkerRecords(t, filepath.Join(fixture.workDir, hostMarkerName), "my-work ec2")
}

func TestEC2Backend_Create_FailsWhenACreationScriptFails(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))
	fixture.workDir = pidYamlDeclaringOneScriptOfEachKind(t)
	fixture.remote.rejects = creationScriptPath

	err := fixture.backend().Create(fixture.createOptions())

	if err == nil {
		t.Fatal("Create() returned nil error for a creation script the instance rejected")
	}
	if !strings.Contains(err.Error(), creationScriptPath) {
		t.Errorf("Create() error = %q, want it to name %q", err.Error(), creationScriptPath)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.workDir, hostMarkerName)); statErr == nil {
		t.Error("Create() ran the host script after a creation script had already failed")
	}
}

func TestEC2Backend_Create_RunsNoScriptWhenTheWorkDirectoryHasNoPidYaml(t *testing.T) {
	fixture := ec2RegionalFixture(t, newHostProvisioningSpy(), ec2FakeTerraform(t))

	if err := fixture.backend().Create(fixture.createOptions()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for _, cmd := range fixture.remote.commands {
		if looksLikeAPidYamlScript(cmd) {
			t.Errorf("Create() ran %v with no pid.yaml to declare it", cmd.Args)
		}
	}
}

// looksLikeAPidYamlScript matches the shape assertScriptRanInTheClone pins — a
// script file run from the clone — and so not the SDKMAN install, which is
// `bash -s` fed on stdin from the home directory.
func looksLikeAPidYamlScript(cmd ec2.RemoteCommand) bool {
	return cmd.Workdir == ec2.RemoteRepoDir && len(cmd.Args) == 2 && cmd.Args[0] == "bash" && cmd.Stdin == ""
}

func lastRemoteCommands(t *testing.T, commands []ec2.RemoteCommand, count int) []ec2.RemoteCommand {
	t.Helper()

	if len(commands) < count {
		t.Fatalf("ran %d remote commands, want at least %d: %v", len(commands), count, commands)
	}
	return commands[len(commands)-count:]
}

// assertScriptRanInTheClone pins where a pid.yaml script runs and the
// environment it sees, since a script run from the wrong directory or without
// ISOLARIUM_TYPE would otherwise look like a success.
func assertScriptRanInTheClone(t *testing.T, cmd ec2.RemoteCommand, path string) {
	t.Helper()

	if cmd.Workdir != ec2.RemoteRepoDir {
		t.Errorf("%s ran from %q, want %q", path, cmd.Workdir, ec2.RemoteRepoDir)
	}
	assertArgsEqual(t, path+" args", cmd.Args, []string{"bash", path})
	if got := cmd.EnvVars["ISOLARIUM_NAME"]; got != "my-work" {
		t.Errorf("%s saw ISOLARIUM_NAME=%q, want %q", path, got, "my-work")
	}
	if got := cmd.EnvVars["ISOLARIUM_TYPE"]; got != "ec2" {
		t.Errorf("%s saw ISOLARIUM_TYPE=%q, want %q", path, got, "ec2")
	}
}

func assertHostMarkerRecords(t *testing.T, path, want string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the host script left no marker at %s: %v", path, err)
	}
	if got := strings.TrimSpace(string(contents)); got != want {
		t.Errorf("the host script recorded %q, want %q", got, want)
	}
}
