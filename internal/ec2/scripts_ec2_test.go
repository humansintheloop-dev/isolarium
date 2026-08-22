//go:build ec2

package ec2_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pidScriptFixtureDir  = "testdata/pidscripts"
	hostMarkerDirEnvVar  = "ISOLARIUM_HOST_MARKER_DIR"
	creationMarkerName   = "creation-ran"
	envMarkerName        = "env-ran"
	hostMarkerName       = "host-ran"
	instanceIsolationTag = "ec2"
)

// hostMarkerDir is where the pid.yaml host script leaves its marker. It is
// resolved once, before the shared instance is created, because the script runs
// during that create rather than during the test that asserts on it.
var hostMarkerDir string

// TestEC2Instance_RunsPidYamlScripts proves the three pid.yaml `ec2` hooks fire
// during a real create: the creation script and the env script inside the
// instance, the host script on the host. Each marker carries the environment its
// script was handed, so a script that ran with the wrong one cannot pass.
func TestEC2Instance_RunsPidYamlScripts(t *testing.T) {
	environment := sharedInstance(t)

	environment.assertInstanceMarkerRecordsTheScriptEnvironment(creationMarkerName)
	environment.assertInstanceMarkerRecordsTheScriptEnvironment(envMarkerName)
	environment.assertHostMarkerRecordsTheScriptEnvironment()
}

// pidScriptWorkDirectory is the work directory the whole suite's create is given,
// so every run exercises the pid.yaml hooks. Resolving it also names the
// directory the host script writes to, which the fixture declares as a required
// environment variable so a lost setting fails the create rather than the
// assertion.
func pidScriptWorkDirectory(t *testing.T) string {
	t.Helper()

	workDir, err := filepath.Abs(pidScriptFixtureDir)
	if err != nil {
		t.Fatalf("resolving %s: %v", pidScriptFixtureDir, err)
	}

	hostMarkerDir, err = os.MkdirTemp("", "isolarium-ec2-host-marker")
	if err != nil {
		t.Fatalf("creating the host marker directory: %v", err)
	}
	if err := os.Setenv(hostMarkerDirEnvVar, hostMarkerDir); err != nil {
		t.Fatalf("setting %s: %v", hostMarkerDirEnvVar, err)
	}
	return workDir
}

func (e *ec2Environment) assertInstanceMarkerRecordsTheScriptEnvironment(marker string) {
	e.t.Helper()

	path := instanceHomeDir + "/repo/" + marker
	exitCode, output := e.askInstance("cat", path)
	if exitCode != 0 {
		e.t.Fatalf("cat %s exited %d, want 0 — the script that writes it never ran", path, exitCode)
	}
	e.assertMarkerRecordsTheScriptEnvironment(marker, output)
}

func (e *ec2Environment) assertHostMarkerRecordsTheScriptEnvironment() {
	e.t.Helper()

	path := filepath.Join(hostMarkerDir, hostMarkerName)
	contents, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("reading %s: %v — the host script never ran", path, err)
	}
	e.assertMarkerRecordsTheScriptEnvironment(hostMarkerName, string(contents))
}

func (e *ec2Environment) assertMarkerRecordsTheScriptEnvironment(marker, contents string) {
	e.t.Helper()

	want := "ISOLARIUM_NAME=" + e.name + " ISOLARIUM_TYPE=" + instanceIsolationTag
	if got := strings.TrimSpace(contents); got != want {
		e.t.Errorf("%s recorded %q, want %q", marker, got, want)
	}
}

// pidScriptMarkers are the files the pid.yaml scripts leave in the clone, which
// every assertion about the clone's cleanliness has to expect.
func pidScriptMarkers() []string {
	return []string{creationMarkerName, envMarkerName}
}
