package status

import (
	"os"
	"testing"
)

func TestGetStatus_ReturnsValidVMState(t *testing.T) {
	s := GetStatus()

	validStates := map[string]bool{"none": true, "running": true, "stopped": true}
	if !validStates[s.VMState] {
		t.Errorf("expected VMState to be 'none', 'running', or 'stopped', got '%s'", s.VMState)
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenNoCredentials(t *testing.T) {
	// Clear env vars to ensure clean state
	os.Unsetenv("GITHUB_APP_ID")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	s := GetStatus()

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false")
	}
}

func TestGetStatus_ReturnsConfiguredWhenBothEnvVarsSet(t *testing.T) {
	// Set both env vars
	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "test-private-key")
	defer func() {
		os.Unsetenv("GITHUB_APP_ID")
		os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	}()

	s := GetStatus()

	if !s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be true when both env vars are set")
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenOnlyAppIDSet(t *testing.T) {
	os.Setenv("GITHUB_APP_ID", "12345")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	defer os.Unsetenv("GITHUB_APP_ID")

	s := GetStatus()

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false when only GITHUB_APP_ID is set")
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenOnlyPrivateKeySet(t *testing.T) {
	os.Unsetenv("GITHUB_APP_ID")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "test-private-key")
	defer os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	s := GetStatus()

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false when only GITHUB_APP_PRIVATE_KEY_PATH is set")
	}
}

func TestStatus_HasRepositoryFields(t *testing.T) {
	s := Status{
		VMState:             "running",
		GitHubAppConfigured: true,
		Repository:          "cer/isolarium",
		Branch:              "main",
	}

	if s.Repository != "cer/isolarium" {
		t.Errorf("expected Repository 'cer/isolarium', got '%s'", s.Repository)
	}
	if s.Branch != "main" {
		t.Errorf("expected Branch 'main', got '%s'", s.Branch)
	}
}
