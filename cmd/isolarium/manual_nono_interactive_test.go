//go:build manual

package main

import (
	"bytes"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

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

func TestClaudeInteractiveInNono_Manual(t *testing.T) {
	binary := buildBinary(t)

	cmd := exec.Command(binary, "--type", "nono", "run", "-i", "--", "claude", "hello")
	cmd.Dir = projectRoot(t)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("failed to start PTY: %v", err)
	}
	defer ptmx.Close()

	output := readPTYOutput(ptmx)
	needle := []byte("Hello!")
	deadline := time.After(15 * time.Second)

	for {
		select {
		case <-deadline:
			_ = cmd.Process.Kill()
			<-output.done
			t.Fatalf("timed out waiting for Claude response containing %q\nBuffer size: %d bytes", needle, output.size())
		case <-time.After(250 * time.Millisecond):
			if output.contains(needle) {
				t.Logf("Claude responded interactively")
				_ = cmd.Process.Kill()
				<-output.done
				return
			}
		}
	}
}
