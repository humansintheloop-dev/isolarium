package nono

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func nonInteractiveCommand(args []string) sandboxCommand {
	return sandboxCommand{args: args, interactive: false}
}

func TestRunWithSignalsReturnsZeroExitCodeForSuccessfulCommand(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	exitCode, err := runWithSignals(nonInteractiveCommand([]string{"echo", "hello"}), sigCh, 10*time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunWithSignalsPropagatesNonZeroExitCode(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	exitCode, err := runWithSignals(nonInteractiveCommand([]string{"sh", "-c", "exit 42"}), sigCh, 10*time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestRunWithSignalsForwardsSIGINTAndExitsWithCode130(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sigCh <- syscall.SIGINT
	}()

	exitCode, err := runWithSignals(nonInteractiveCommand([]string{"sleep", "100"}), sigCh, 10*time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 130 {
		t.Errorf("expected exit code 130, got %d", exitCode)
	}
}

func TestRunWithSignalsSendsKillAfterGracePeriodWhenChildIgnoresSignal(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sigCh <- syscall.SIGINT
	}()

	start := time.Now()
	exitCode, err := runWithSignals(nonInteractiveCommand([]string{"sh", "-c", `trap "" INT; sleep 100`}), sigCh, 1*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 130 {
		t.Errorf("expected exit code 130, got %d", exitCode)
	}
	if elapsed > 3*time.Second {
		t.Errorf("expected process to terminate within ~2 seconds, took %v", elapsed)
	}
}

func TestRunWithSignalsSendsImmediateKillOnSecondSignalDuringGracePeriod(t *testing.T) {
	sigCh := make(chan os.Signal, 2)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sigCh <- syscall.SIGINT
		time.Sleep(200 * time.Millisecond)
		sigCh <- syscall.SIGINT
	}()

	start := time.Now()
	exitCode, err := runWithSignals(nonInteractiveCommand([]string{"sh", "-c", `trap "" INT; sleep 100`}), sigCh, 30*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 130 {
		t.Errorf("expected exit code 130, got %d", exitCode)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected process to terminate within ~1 second, took %v", elapsed)
	}
}

func TestRunWithSignalsForwardsSIGTERMAndExitsWithCode143(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sigCh <- syscall.SIGTERM
	}()

	exitCode, err := runWithSignals(nonInteractiveCommand([]string{"sleep", "100"}), sigCh, 10*time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 143 {
		t.Errorf("expected exit code 143, got %d", exitCode)
	}
}
