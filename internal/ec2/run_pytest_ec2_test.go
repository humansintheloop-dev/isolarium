//go:build ec2

// The file name is load-bearing. It sorts after repo_ec2_test.go, because
// uv run writes testdata/python-cli-app/.venv into the clone and
// repo_ec2_test.go asserts the clone is clean; and ahead of tmux_ec2_test.go,
// whose tests deliberately leave a process running in the shared tmux session
// these runs need free.
package ec2_test

import (
	"strings"
	"testing"
	"time"
)

const (
	pythonCliAppDir = "testdata/python-cli-app"
	pyprojectPath   = pythonCliAppDir + "/pyproject.toml"
	pytestPassed    = "2 passed"
	greeterGreeting = "Hello, EC2!"

	// uvRunTimeout allows for the first run's download of a Python interpreter
	// and of every dependency, on top of the tests themselves.
	uvRunTimeout = 10 * time.Minute
)

// TestEC2Run_PytestPassesInThePythonCliApp proves the uv the instance carries
// is enough for a real workload: the Python app that arrives with the clone of
// this repository has its tests run under uv, the way
// cmd/isolarium/e2e_pytest_vm_test.go runs them in a VM.
func TestEC2Run_PytestPassesInThePythonCliApp(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()
	environment.assertTheCloneCarriesThePythonCliApp()

	started := time.Now()
	run := environment.startIsolariumRun(inThePythonCliApp("rm -rf .venv && uv run pytest -v")...)
	exitCode := run.waitUntilItEnds(uvRunTimeout)
	environment.t.Logf("TIMING: uv run pytest took %s", time.Since(started).Round(time.Second))

	assertRunSucceededPrinting(t, run, exitCode, pytestPassed)
}

func TestEC2Run_GreeterCliPrintsAGreeting(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()
	environment.assertTheCloneCarriesThePythonCliApp()

	run := environment.startIsolariumRun(inThePythonCliApp("uv run greeter EC2")...)
	exitCode := run.waitUntilItEnds(uvRunTimeout)

	assertRunSucceededPrinting(t, run, exitCode, greeterGreeting)
}

// inThePythonCliApp needs no PATH prefix for uv, unlike the VM test: cloud-init
// writes /home/ubuntu/.local/bin, where the uv installer puts the binary, into
// the system PATH, and toolchain_ec2_test.go proves `uv --version` resolves in
// the same non-interactive shell the run uses.
func inThePythonCliApp(command string) []string {
	return []string{"bash", "-c", "cd " + pythonCliAppDir + " && " + command}
}

func assertRunSucceededPrinting(t *testing.T, run *isolariumRun, exitCode int, want string) {
	t.Helper()

	if exitCode != 0 {
		t.Errorf("%s exited %d, want 0\n%s", run.label, exitCode, visibleText(run.output()))
	}
	if !strings.Contains(visibleText(run.output()), want) {
		t.Errorf("%s did not print %q:\n%s", run.label, want, visibleText(run.output()))
	}
}

// assertTheCloneCarriesThePythonCliApp fails with a clear message when the
// checkout on the instance is missing the app, rather than leaving that to a
// uv error deep in the run's output.
func (e *ec2Environment) assertTheCloneCarriesThePythonCliApp() {
	e.t.Helper()

	if exitCode, _ := e.askInstanceInRepo("test", "-f", pyprojectPath); exitCode != 0 {
		e.t.Fatalf("%s is missing from the clone on the instance; the checkout did not bring the Python CLI app", pyprojectPath)
	}
}
