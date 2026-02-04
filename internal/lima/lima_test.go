package lima

import (
	"strings"
	"testing"
)

func TestGenerateConfig(t *testing.T) {
	config, err := GenerateConfig()
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	// Verify essential components are present in the configuration
	if !strings.Contains(config, `arch: "aarch64"`) && !strings.Contains(config, `arch: "x86_64"`) {
		t.Error("config should specify architecture")
	}

	// Verify no host mounts are configured (security requirement)
	if strings.Contains(config, "mounts:") {
		// If mounts section exists, verify it's empty or explicitly disabled
		if !strings.Contains(config, "mounts: []") {
			t.Error("config should not have host mounts for security")
		}
	}

	// Verify Docker is configured to be installed
	if !strings.Contains(config, "docker") {
		t.Error("config should include Docker installation")
	}
}

func TestGetVMName(t *testing.T) {
	name := GetVMName()
	if name != "isolarium" {
		t.Errorf("expected VM name 'isolarium', got %q", name)
	}
}
