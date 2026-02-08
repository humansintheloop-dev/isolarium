package lima

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed template.yaml
var templateYAML string

const vmName = "isolarium"

// GetVMName returns the name of the isolarium VM
func GetVMName() string {
	return vmName
}

// GenerateConfig returns the Lima VM configuration YAML
func GenerateConfig() (string, error) {
	return templateYAML, nil
}

// VMExists checks if the isolarium VM already exists
func VMExists() (bool, error) {
	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		// limactl not installed or not working
		return false, fmt.Errorf("failed to check VM status: %w", err)
	}
	// Check if output contains our VM name
	return containsVM(string(output), vmName), nil
}

// containsVM checks if the JSON output from limactl list contains the VM
func containsVM(jsonOutput, name string) bool {
	// Simple string check - the VM name will appear in the JSON if it exists
	return len(jsonOutput) > 2 && // More than just "[]"
		(strings.Contains(jsonOutput, fmt.Sprintf(`"name":"%s"`, name)) ||
			strings.Contains(jsonOutput, fmt.Sprintf(`"name": "%s"`, name)))
}

// parseVMState extracts the VM state from limactl list JSON output.
// Returns "running", "stopped", or "none".
func parseVMState(jsonOutput, name string) string {
	// Each line is a separate JSON object for a VM
	for _, line := range strings.Split(jsonOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Check if this line is for our VM
		if !strings.Contains(line, fmt.Sprintf(`"name":"%s"`, name)) &&
			!strings.Contains(line, fmt.Sprintf(`"name": "%s"`, name)) {
			continue
		}
		// Extract status
		if strings.Contains(line, `"status":"Running"`) || strings.Contains(line, `"status": "Running"`) {
			return "running"
		}
		if strings.Contains(line, `"status":"Stopped"`) || strings.Contains(line, `"status": "Stopped"`) {
			return "stopped"
		}
	}
	return "none"
}

// GetVMState returns the current state of the isolarium VM: "running", "stopped", or "none"
func GetVMState() string {
	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "none"
	}
	return parseVMState(string(output), vmName)
}

// CreateVM creates and starts the isolarium Lima VM
func CreateVM() error {
	// Check if VM already exists
	exists, err := VMExists()
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("VM '%s' already exists", vmName)
	}

	// Write config to a temporary file
	tmpDir, err := os.MkdirTemp("", "isolarium-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "lima.yaml")
	config, err := GenerateConfig()
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Create the VM using limactl
	cmd := exec.Command("limactl", "create", "--name", vmName, configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	// Start the VM
	cmd = exec.Command("limactl", "start", vmName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	return nil
}

// DestroyVM stops and deletes the isolarium Lima VM
func DestroyVM() error {
	exists, err := VMExists()
	if err != nil {
		return err
	}
	if !exists {
		return nil // Idempotent - no error if VM doesn't exist
	}

	// Stop the VM first (ignore errors if already stopped)
	stopCmd := exec.Command("limactl", "stop", vmName)
	stopCmd.Run() // Ignore error

	// Delete the VM
	cmd := exec.Command("limactl", "delete", vmName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	return nil
}

// GetVMHomeDir returns the home directory of the default user inside the VM
func GetVMHomeDir() (string, error) {
	cmd := exec.Command("limactl", "shell", vmName, "--", "bash", "-c", "echo $HOME")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get VM home directory: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetArchitecture returns the architecture string for Lima
func GetArchitecture() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}
