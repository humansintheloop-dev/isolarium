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

type testEnv struct {
	t       *testing.T
	binary  string
	envType string
	root    string
}

func newTestEnv(t *testing.T, envType string) testEnv {
	t.Helper()
	root := projectRoot(t)
	binaryPath := filepath.Join(root, "bin", "isolarium")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/isolarium")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}
	return testEnv{t: t, binary: binaryPath, envType: envType, root: root}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func (e testEnv) ensureReady() {
	e.t.Helper()
	if e.envType == "nono" {
		return
	}
	state := e.state()
	if state == "running" {
		return
	}
	if state == "stopped" {
		e.destroy()
	}
	e.create()
}

func (e testEnv) state() string {
	e.t.Helper()
	defaultNames := map[string]string{"container": "isolarium-container", "vm": "isolarium"}
	name := defaultNames[e.envType]

	cmd := exec.Command(e.binary, "--type", e.envType, "status")
	output, _ := cmd.Output()
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == name {
			return fields[2]
		}
	}
	return "none"
}

func (e testEnv) destroy() {
	e.t.Helper()
	e.t.Logf("destroying stopped %s environment...", e.envType)
	cmd := exec.Command(e.binary, "--type", e.envType, "destroy")
	cmd.Dir = e.root
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Logf("destroy failed (continuing): %v\n%s", err, out)
	}
}

func (e testEnv) create() {
	e.t.Helper()
	e.t.Logf("creating %s environment...", e.envType)
	createArgs := []string{"--type", e.envType}
	createArgs = append(createArgs, envFileArgs(e.t, e.root)...)
	createArgs = append(createArgs, "create")
	cmd := exec.Command(e.binary, createArgs...)
	cmd.Dir = e.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("failed to create %s environment: %v\n%s", e.envType, err, out)
	}
}

func (e testEnv) runClaude() string {
	e.t.Helper()
	cmd := exec.Command(e.binary, "--type", e.envType, "run", "--", "claude", "-p", "hello")
	cmd.Dir = e.root
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("isolarium --type %s run -- claude -p hello failed: %v\noutput: %s", e.envType, err, output)
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

func (e testEnv) runClaudeInteractive() {
	e.t.Helper()

	cmd := exec.Command(e.binary, "--type", e.envType, "run", "-i", "--", "claude", "hello")
	cmd.Dir = e.root
	ptmx, err := pty.Start(cmd)
	if err != nil {
		e.t.Fatalf("failed to start PTY: %v", err)
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
			e.t.Fatalf("timed out waiting for Claude interactive response\nBuffer size: %d bytes\nRaw: %s", len(raw), raw)
		case <-time.After(250 * time.Millisecond):
			for _, needle := range acceptedResponses {
				if output.contains(needle) {
					e.t.Logf("Claude responded interactively (matched %q)", needle)
					_ = cmd.Process.Kill()
					<-output.done
					return
				}
			}
		}
	}
}
