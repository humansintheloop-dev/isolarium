//go:build ec2

// This file holds no test, so its name plays no part in the order the suite
// runs in. It is where the suite drives the built binary from: the create that
// launches the shared instance goes through `isolarium run --create`, and the
// run, status, destroy and wipe tests each drive the same binary a user would.
package ec2_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// builtIsolariumBinary is the CLI under test, compiled once per run so every
// test drives the same binary a user would.
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

// processEnvironment is the environment one invocation of the built binary runs
// with. It is its own type so that the helpers which strip credentials out of it
// cannot be handed any slice of strings that happens to be at hand.
type processEnvironment []string

// binaryEnvironment points the built binary at the home directory the suite's
// metadata lives under, since the CLI derives ~/.isolarium for itself. Moving
// HOME also moves where git looks for the host's global config, and create reads
// user.email and user.name from there for the instance's clone, so the host's
// own file is named explicitly.
func binaryEnvironment() processEnvironment {
	return append(os.Environ(), "HOME="+sharedHomeDir, "GIT_CONFIG_GLOBAL="+hostGitConfigPath)
}

// hostGitConfigPath is the global git config the host keeps its author identity
// in, resolved once by TestMain so that a host without one fails the suite
// before any instance is launched rather than minutes into create.
var hostGitConfigPath string

// resolveHostGitConfigPath asks git which file user.email comes from, rather
// than assuming ~/.gitconfig, so a host that keeps its identity under
// $XDG_CONFIG_HOME/git or in an included file is handed the right one.
func resolveHostGitConfigPath() (string, error) {
	output, err := exec.Command("git", "config", "--global", "--show-origin", "--get", "user.email").Output()
	if err != nil {
		return "", fmt.Errorf("resolving the host's global git config: git config --global --show-origin --get user.email: %w", err)
	}
	origin, _, found := strings.Cut(strings.TrimSpace(string(output)), "\t")
	if !found || !strings.HasPrefix(origin, gitConfigFileOrigin) {
		return "", fmt.Errorf("resolving the host's global git config: unexpected origin %q", origin)
	}
	return strings.TrimPrefix(origin, gitConfigFileOrigin), nil
}

const gitConfigFileOrigin = "file:"

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

// runIsolarium drives the built binary from the suite's work directory — the
// pid.yaml fixture inside this checkout, which is what gives the binary a git
// repository to resolve and a pid.yaml to honour — and hands back its exit code
// with everything it printed. A binary that could not be started at all is a
// failure of the suite rather than a status worth returning.
func (e *ec2Environment) runIsolarium(args ...string) (int, string) {
	e.t.Helper()

	command := exec.Command(isolariumBinary(e.t), args...)
	command.Dir = e.workDir
	command.Env = binaryEnvironment()
	output, err := runBinaryStreaming(command)

	exitCode, err := binaryExitCode(err)
	if err != nil {
		e.t.Fatalf("isolarium %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return exitCode, output
}

// binaryExitCode separates a binary that ran and exited — whose status is the
// thing under test — from one the suite could not run at all.
func binaryExitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

// runArgs is the invocation i2code uses, with the global flags ahead of the
// subcommand: `isolarium --name <name> --type ec2 run [--create] -- <command>`.
// The credential copy is left out because it would read the host's keychain,
// which the suite may not have; TestEC2Instance_ClaudeAuthenticates proves that
// copy on its own.
func (e *ec2Environment) runArgs(create bool, command ...string) []string {
	args := []string{"--name", e.name, "--type", instanceIsolationTag, "run", "--copy-session=false"}
	if create {
		args = append(args, "--create")
	}
	args = append(args, "--")
	return append(args, command...)
}
