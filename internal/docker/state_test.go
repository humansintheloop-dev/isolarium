package docker

import (
	"fmt"
	"testing"

	"github.com/cer/isolarium/internal/command"
)

func TestBuildInspectCommandProducesCorrectDockerInspectArgs(t *testing.T) {
	args := BuildInspectCommand("my-container")
	expected := []string{"docker", "inspect", "--format", "{{.State.Status}}", "my-container"}
	if len(args) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Fatalf("expected args[%d] = %q, got %q", i, v, args[i])
		}
	}
}

func TestParseContainerStateReturnsRunningForRunningContainer(t *testing.T) {
	state := ParseContainerState("running\n")
	if state != "running" {
		t.Errorf("expected %q, got %q", "running", state)
	}
}

func TestParseContainerStateReturnsStoppedForExitedContainer(t *testing.T) {
	state := ParseContainerState("exited\n")
	if state != "stopped" {
		t.Errorf("expected %q, got %q", "stopped", state)
	}
}

func TestParseContainerStateReturnsNoneForEmptyOutput(t *testing.T) {
	state := ParseContainerState("")
	if state != "none" {
		t.Errorf("expected %q, got %q", "none", state)
	}
}

func TestGetStateReturnsRunningForRunningContainer(t *testing.T) {
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "my-container").Returns("running\n")

	checker := &StateChecker{Runner: runner}
	state := checker.GetState("my-container")

	if state != "running" {
		t.Errorf("expected %q, got %q", "running", state)
	}
	runner.VerifyExecuted()
}

func TestGetStateReturnsNoneWhenContainerDoesNotExist(t *testing.T) {
	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "inspect", "--format", "{{.State.Status}}", "missing-container").Fails(fmt.Errorf("No such object: missing-container"))

	checker := &StateChecker{Runner: runner}
	state := checker.GetState("missing-container")

	if state != "none" {
		t.Errorf("expected %q, got %q", "none", state)
	}
}
