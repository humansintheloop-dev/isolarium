package nono

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestRunWithSignalsReturnsZeroExitCodeForSuccessfulCommand(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	exitCode, err := runWithSignals([]string{"echo", "hello"}, nil, sigCh, 10*time.Second, false)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunWithSignalsPropagatesNonZeroExitCode(t *testing.T) {
	sigCh := make(chan os.Signal, 1)

	exitCode, err := runWithSignals([]string{"sh", "-c", "exit 42"}, nil, sigCh, 10*time.Second, false)

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

	exitCode, err := runWithSignals([]string{"sleep", "100"}, nil, sigCh, 10*time.Second, false)

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
	exitCode, err := runWithSignals([]string{"sh", "-c", `trap "" INT; sleep 100`}, nil, sigCh, 1*time.Second, false)
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
	exitCode, err := runWithSignals([]string{"sh", "-c", `trap "" INT; sleep 100`}, nil, sigCh, 30*time.Second, false)
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

	exitCode, err := runWithSignals([]string{"sleep", "100"}, nil, sigCh, 10*time.Second, false)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 143 {
		t.Errorf("expected exit code 143, got %d", exitCode)
	}
}
