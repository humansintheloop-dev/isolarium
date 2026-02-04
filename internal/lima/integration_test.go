// +build integration

package lima

import (
	"os/exec"
	"strings"
	"testing"
)

// Integration tests for Lima VM management
// These tests require Lima to be installed and can take several minutes to run
// Run with: go test -tags=integration ./internal/lima/...

func TestCreateAndDestroyVM_Integration(t *testing.T) {
	// Skip if Lima is not installed
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("Lima not installed, skipping integration test")
	}

	// Clean up any existing VM first
	_ = DestroyVM()

	// Create VM
	err := CreateVM()
	if err != nil {
		t.Fatalf("CreateVM failed: %v", err)
	}

	// Verify VM exists and is running
	cmd := exec.Command("limactl", "list", "--format", "{{.Name}}:{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to list VMs: %v", err)
	}
	if !strings.Contains(string(output), "isolarium:Running") {
		t.Errorf("expected VM to be running, got: %s", output)
	}

	// Clean up
	err = DestroyVM()
	if err != nil {
		t.Fatalf("DestroyVM failed: %v", err)
	}

	// Verify VM is gone
	output, _ = exec.Command("limactl", "list", "--format", "{{.Name}}").Output()
	if strings.Contains(string(output), "isolarium") {
		t.Errorf("VM still exists after destroy: %s", output)
	}
}

func TestVMHasRequiredTools_Integration(t *testing.T) {
	// Skip if Lima is not installed
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("Lima not installed, skipping integration test")
	}

	// This test assumes VM is already created (run after TestCreateAndDestroyVM_Integration)
	// Check for required tools (all should be in PATH via symlinks or direct install)
	tools := []string{"git", "node", "docker", "gh", "claude", "java"}
	for _, tool := range tools {
		cmd := exec.Command("limactl", "shell", "isolarium", "--", "which", tool)
		if err := cmd.Run(); err != nil {
			t.Errorf("tool %s not found in VM", tool)
		}
	}

	// Check JAVA_HOME is set in /etc/environment
	cmd := exec.Command("limactl", "shell", "isolarium", "--", "grep", "JAVA_HOME", "/etc/environment")
	if err := cmd.Run(); err != nil {
		t.Error("JAVA_HOME not set in /etc/environment")
	}
}
