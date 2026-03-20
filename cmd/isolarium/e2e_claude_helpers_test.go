//go:build e2e_claude

package main

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func buildClaudeBinary(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	binaryPath := filepath.Join(root, "bin", "isolarium")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/isolarium")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}
	return binaryPath
}

func ensureEnvironmentReady(t *testing.T, binary, envType string) {
	t.Helper()
	if envType == "nono" {
		return
	}
	cmd := exec.Command(binary, "--type", envType, "status")
	output, _ := cmd.Output()
	if strings.Contains(string(output), "running") {
		return
	}
	t.Logf("creating %s environment...", envType)
	root := projectRoot(t)
	createArgs := []string{"--type", envType}
	createArgs = append(createArgs, envFileArgs(t, root)...)
	createArgs = append(createArgs, "create")
	cmd = exec.Command(binary, createArgs...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to create %s environment: %v\n%s", envType, err, out)
	}
}

func claudeInIsolarium(t *testing.T, envType string) string {
	t.Helper()
	binary := buildClaudeBinary(t)
	ensureEnvironmentReady(t, binary, envType)
	cmd := exec.Command(binary, "--type", envType, "run", "--", "claude", "-p", "hello")
	cmd.Dir = projectRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolarium --type %s run -- claude -p hello failed: %v\noutput: %s", envType, err, output)
	}
	return string(output)
}

func verifyClaudeResponded(t *testing.T, output string) {
	t.Helper()
	t.Logf("Claude response:\n%s", output)
	trimmed := strings.TrimSpace(output)
	if len(trimmed) == 0 {
		t.Fatal("expected non-empty response from claude")
	}
}

type ptyOutput struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func readPTYOutput(r io.Reader) *ptyOutput {
	out := &ptyOutput{done: make(chan struct{})}
	go func() {
		tmp := make([]byte, 1024)
		for {
			n, err := r.Read(tmp)
			if n > 0 {
				out.mu.Lock()
				out.buf.Write(tmp[:n])
				out.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
		close(out.done)
	}()
	return out
}

func (o *ptyOutput) contains(needle []byte) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Contains(o.buf.Bytes(), needle)
}

func (o *ptyOutput) size() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Len()
}

func claudeInteractiveInIsolarium(t *testing.T, envType string) {
	t.Helper()
	binary := buildClaudeBinary(t)
	ensureEnvironmentReady(t, binary, envType)

	cmd := exec.Command(binary, "--type", envType, "run", "-i", "--", "claude", "hello")
	cmd.Dir = projectRoot(t)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("failed to start PTY: %v", err)
	}
	defer ptmx.Close()

	output := readPTYOutput(ptmx)
	acceptedResponses := [][]byte{
		[]byte("Hello!"),
		[]byte("Choose"),
	}
	deadline := time.After(15 * time.Second)

	for {
		select {
		case <-deadline:
			_ = cmd.Process.Kill()
			<-output.done
			output.mu.Lock()
			raw := output.buf.String()
			output.mu.Unlock()
			t.Fatalf("timed out waiting for Claude interactive response\nBuffer size: %d bytes\nRaw: %s", len(raw), raw)
		case <-time.After(250 * time.Millisecond):
			for _, needle := range acceptedResponses {
				if output.contains(needle) {
					t.Logf("Claude responded interactively (matched %q)", needle)
					_ = cmd.Process.Kill()
					<-output.done
					return
				}
			}
		}
	}
}
