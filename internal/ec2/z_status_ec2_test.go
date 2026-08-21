//go:build ec2

// The z_ prefix is load-bearing, in the same way the zz_ prefix on
// zz_lifecycle_terminate_ec2_test.go is. go test walks a package's test files in
// sorted-filename order, and this file has to come after every test that needs
// the shared instance alive, because the second of its tests destroys it — while
// still coming before the terminate test, which has nothing left to do once the
// destroy here has already happened.
package ec2_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	statusHeaderName           = "NAME"
	statusEmptyLine            = "No environments found"
	unknownState               = "unknown"
	runningState               = "running"
	statusColumnsBeforeDetails = 3
)

// TestEC2Instance_StatusDegradesWithoutCredentials runs ahead of
// TestEC2Instance_StatusReportsRunningThenGone because that test destroys the
// shared instance, and an environment AWS can no longer be asked about would
// satisfy this assertion for the wrong reason.
func TestEC2Instance_StatusDegradesWithoutCredentials(t *testing.T) {
	environment := sharedInstance(t)

	environment.assertStatusReportsUnknownWithoutAWSCredentials()
}

// TestEC2Instance_StatusReportsRunningThenGone proves the status command reads
// the real account rather than the local metadata file: the same environment is
// reported as running while its instance exists, and stops being reported at all
// once destroy has taken it away.
func TestEC2Instance_StatusReportsRunningThenGone(t *testing.T) {
	environment := sharedInstance(t)

	environment.assertStatusReportsRunning()
	environment.destroyThroughTheCLI()
	environment.assertStatusReportsNoEC2Environment()
}

func (e *ec2Environment) assertStatusReportsRunning() {
	e.t.Helper()

	row := e.requireEC2StatusRow(binaryEnvironment())
	if row.state != runningState {
		e.t.Errorf("isolarium status state for %s = %q, want %q", e.name, row.state, runningState)
	}
	if row.details != e.expectedStatusDetails() {
		e.t.Errorf("isolarium status details for %s = %q, want %q", e.name, row.details, e.expectedStatusDetails())
	}
}

func (e *ec2Environment) expectedStatusDetails() string {
	return fmt.Sprintf("%s/%s (%s)", e.repository.Owner, e.repository.Repo, e.repository.Branch)
}

// assertStatusReportsUnknownWithoutAWSCredentials asks for the state of an
// instance that is demonstrably still there, so an unknown can only have come
// from the lookup the missing credentials prevented.
func (e *ec2Environment) assertStatusReportsUnknownWithoutAWSCredentials() {
	e.t.Helper()

	row := e.requireEC2StatusRow(withoutAWSCredentials(binaryEnvironment()))
	if row.state != unknownState {
		e.t.Errorf("isolarium status state for %s without AWS credentials = %q, want %q", e.name, row.state, unknownState)
	}
}

func (e *ec2Environment) assertStatusReportsNoEC2Environment() {
	e.t.Helper()

	rows := e.runIsolariumStatus(binaryEnvironment())
	if row, found := findEC2Row(rows, e.name); found {
		e.t.Errorf("isolarium status still reports %s as %q after destroy", e.name, row.state)
	}
}

func (e *ec2Environment) requireEC2StatusRow(environment processEnvironment) statusRow {
	e.t.Helper()

	rows := e.runIsolariumStatus(environment)
	row, found := findEC2Row(rows, e.name)
	if !found {
		e.t.Fatalf("isolarium status reported no ec2 row for %s; rows were %v", e.name, rows)
	}
	return row
}

// destroyThroughTheCLI tears the shared instance down the way the observable
// describes it, and records that it is gone so neither the terminate test nor
// the suite teardown pays to destroy it a second time.
func (e *ec2Environment) destroyThroughTheCLI() {
	e.t.Helper()

	destroy := exec.Command(isolariumBinary(e.t), "destroy", "--type", "ec2", "--name", e.name)
	destroy.Env = binaryEnvironment()
	output, err := runBinaryStreaming(destroy)
	if err != nil {
		e.t.Fatalf("isolarium destroy --type ec2 --name %s: %v\n%s", e.name, err, output)
	}
	e.destroyed = true
}

// runIsolariumStatus drives the built binary and hands back what it printed,
// broken back into columns. A non-zero exit is a failure of the command itself,
// which is precisely what the credential-less case exists to rule out.
func (e *ec2Environment) runIsolariumStatus(environment processEnvironment) []statusRow {
	e.t.Helper()

	status := exec.Command(isolariumBinary(e.t), "status")
	status.Env = environment
	output, err := status.Output()
	if err != nil {
		e.t.Fatalf("isolarium status exited non-zero: %v\n%s", err, output)
	}
	return parseStatusRows(string(output))
}

// statusRow is one line of `isolarium status` output, split back into the
// columns the tabwriter padded apart.
type statusRow struct {
	name    string
	envType string
	state   string
	details string
}

func parseStatusRows(output string) []statusRow {
	var rows []statusRow
	for _, line := range strings.Split(output, "\n") {
		if row, isRow := parseStatusRow(line); isRow {
			rows = append(rows, row)
		}
	}
	return rows
}

// parseStatusRow reports the header and the empty-listing notice as no row at
// all, so that neither can be mistaken for an environment named NAME or No.
func parseStatusRow(line string) (statusRow, bool) {
	fields := strings.Fields(line)
	if len(fields) < statusColumnsBeforeDetails {
		return statusRow{}, false
	}
	if fields[0] == statusHeaderName {
		return statusRow{}, false
	}
	if strings.TrimSpace(line) == statusEmptyLine {
		return statusRow{}, false
	}
	return statusRow{
		name:    fields[0],
		envType: fields[1],
		state:   fields[2],
		details: strings.Join(fields[statusColumnsBeforeDetails:], " "),
	}, true
}

func findEC2Row(rows []statusRow, name string) (statusRow, bool) {
	for _, row := range rows {
		if row.name == name && row.envType == instanceIsolationTag {
			return row, true
		}
	}
	return statusRow{}, false
}

// runBinaryStreaming runs one isolarium invocation, copying its output to the
// suite's own stderr as it arrives, and returns what it printed. A command that
// spends minutes inside terraform would otherwise say nothing until it exited,
// which is indistinguishable from a hang.
func runBinaryStreaming(binary *exec.Cmd) (string, error) {
	var captured bytes.Buffer
	binary.Stdout = io.MultiWriter(&captured, os.Stderr)
	binary.Stderr = binary.Stdout

	err := binary.Run()
	return captured.String(), err
}

// processEnvironment is the environment one invocation of the built binary runs
// with. It is its own type so that the helpers which strip credentials out of it
// cannot be handed any slice of strings that happens to be at hand.
type processEnvironment []string

// binaryEnvironment points the built binary at the home directory the suite's
// metadata lives under, since the CLI derives ~/.isolarium for itself.
func binaryEnvironment() processEnvironment {
	return append(os.Environ(), "HOME="+sharedHomeDir)
}

// withoutAWSCredentials clears the credentials for one invocation only, leaving
// the surrounding suite — which still has an account to talk to — untouched.
func withoutAWSCredentials(environment processEnvironment) processEnvironment {
	var cleared processEnvironment
	for _, variable := range environment {
		if namesAWSCredentials(variable) {
			continue
		}
		cleared = append(cleared, variable)
	}
	return cleared
}

// namesAWSCredentials covers every variable the SDK could still resolve an
// identity from, because leaving any one of them behind would let the lookup
// succeed and report a state rather than the unknown under test.
func namesAWSCredentials(variable string) bool {
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE"} {
		if strings.HasPrefix(variable, name+"=") {
			return true
		}
	}
	return false
}

// builtIsolariumBinary is the CLI under test, compiled once per run so both
// tests drive the same binary a user would.
var builtIsolariumBinary string

func isolariumBinary(t *testing.T) string {
	t.Helper()

	if builtIsolariumBinary != "" {
		return builtIsolariumBinary
	}

	path := filepath.Join(sharedHomeDir, "isolarium")
	build := exec.Command("go", "build", "-o", path, "./cmd/isolarium")
	build.Dir = repositoryCheckout(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the isolarium binary: %v\n%s", err, output)
	}
	builtIsolariumBinary = path
	return builtIsolariumBinary
}
