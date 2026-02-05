//go:build cleanup

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDestroyCommand_SucceedsWhenVMExists(t *testing.T) {
	// Get the current working directory to build absolute path
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "isolarium", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	binaryPath := filepath.Join(cwd, "isolarium")

	// Run destroy command (VM exists from previous tests)
	cmd := exec.Command(binaryPath, "destroy")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("destroy command failed: %v, output: %s", err, output)
	}

	// Verify VM is gone
	listCmd := exec.Command("limactl", "list", "--format", "{{.Name}}")
	listOutput, _ := listCmd.Output()
	if strings.Contains(string(listOutput), "isolarium") {
		t.Error("VM still exists after destroy")
	}
}
