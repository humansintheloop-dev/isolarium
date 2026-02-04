package status

import (
	"testing"
)

func TestGetStatus_ReturnsNoVMWhenNoVMExists(t *testing.T) {
	s := GetStatus()

	if s.VMState != "none" {
		t.Errorf("expected VMState to be 'none', got '%s'", s.VMState)
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenNoCredentials(t *testing.T) {
	s := GetStatus()

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false")
	}
}
