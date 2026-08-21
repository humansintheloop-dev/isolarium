package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecRunner_RunStreaming_WritesTheOutputAsItArrivesAndStillReturnsIt(t *testing.T) {
	var streamed bytes.Buffer

	returned, err := ExecRunner{}.RunStreaming(&streamed, "sh", "-c", "echo to-stdout; echo to-stderr >&2")

	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	for _, want := range []string{"to-stdout", "to-stderr"} {
		if !strings.Contains(streamed.String(), want) {
			t.Errorf("streamed = %q, want it to contain %q", streamed.String(), want)
		}
		if !strings.Contains(string(returned), want) {
			t.Errorf("returned = %q, want it to contain %q", string(returned), want)
		}
	}
}

func TestExecRunner_RunStreaming_ToleratesNoWriter(t *testing.T) {
	returned, err := ExecRunner{}.RunStreaming(nil, "sh", "-c", "echo quiet")

	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if !strings.Contains(string(returned), "quiet") {
		t.Errorf("returned = %q, want it to contain %q", string(returned), "quiet")
	}
}

func TestExecRunner_RunStreaming_ReturnsTheOutputOfAFailedCommand(t *testing.T) {
	var streamed bytes.Buffer

	returned, err := ExecRunner{}.RunStreaming(&streamed, "sh", "-c", "echo before-the-failure; exit 3")

	if err == nil {
		t.Fatal("RunStreaming() returned nil error for a command that exited 3")
	}
	if !strings.Contains(string(returned), "before-the-failure") {
		t.Errorf("returned = %q, want the output the command produced before failing", string(returned))
	}
}

func TestExecRunner_Run_StaysSilent(t *testing.T) {
	returned, err := ExecRunner{}.Run("sh", "-c", "echo secret-material")

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(string(returned), "secret-material") {
		t.Errorf("Run() = %q, want it to capture the output", string(returned))
	}
}
