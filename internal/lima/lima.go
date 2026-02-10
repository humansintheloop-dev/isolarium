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

func VMExists(name string) (bool, error) {
	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		// limactl not installed or not working
		return false, fmt.Errorf("failed to check VM status: %w", err)
	}
	// Check if output contains our VM name
	return containsVM(string(output), name), nil
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

func GetVMState(name string) string {
	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "none"
	}
	return parseVMState(string(output), name)
}

func StartVM(name string) error {
	cmd := exec.Command("limactl", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}
	return nil
}

func CreateVM(name string) error {
	exists, err := VMExists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("VM '%s' already exists", name)
	}

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

	cmd := exec.Command("limactl", "create", "--name", name, configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	cmd = exec.Command("limactl", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	return nil
}

func DestroyVM(name string) error {
	exists, err := VMExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return nil // Idempotent - no error if VM doesn't exist
	}

	stopCmd := exec.Command("limactl", "stop", name)
	stopCmd.Run() // Ignore error

	cmd := exec.Command("limactl", "delete", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	if err := CleanupHostMetadata(name); err != nil {
		return fmt.Errorf("failed to cleanup host metadata: %w", err)
	}

	return nil
}

func GetVMHomeDir(name string) (string, error) {
	cmd := exec.Command("limactl", "shell", name, "--", "bash", "-c", "echo $HOME")
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
