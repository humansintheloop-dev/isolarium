package status

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetStatus_ReturnsValidVMState(t *testing.T) {
	s := GetStatus("isolarium")

	validStates := map[string]bool{"none": true, "running": true, "stopped": true}
	if !validStates[s.VMState] {
		t.Errorf("expected VMState to be 'none', 'running', or 'stopped', got '%s'", s.VMState)
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenNoCredentials(t *testing.T) {
	// Clear env vars to ensure clean state
	os.Unsetenv("GITHUB_APP_ID")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	s := GetStatus("isolarium")

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

	s := GetStatus("isolarium")

	if !s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be true when both env vars are set")
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenOnlyAppIDSet(t *testing.T) {
	os.Setenv("GITHUB_APP_ID", "12345")
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	defer os.Unsetenv("GITHUB_APP_ID")

	s := GetStatus("isolarium")

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false when only GITHUB_APP_ID is set")
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenOnlyPrivateKeySet(t *testing.T) {
	os.Unsetenv("GITHUB_APP_ID")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "test-private-key")
	defer os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	s := GetStatus("isolarium")

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

func TestListAllEnvironments_ReturnsBothVMAndContainer(t *testing.T) {
	baseDir := t.TempDir()

	createVMMetadata(t, baseDir, "my-vm")
	createContainerMetadata(t, baseDir, "my-container", "/home/user/repo")

	stateProvider := func(name, envType string) string {
		return "running"
	}

	envs := ListAllEnvironments(baseDir, stateProvider)

	if len(envs) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(envs))
	}

	assertContainsEnvironment(t, envs, "my-vm", "vm", "running")
	assertContainsEnvironment(t, envs, "my-container", "container", "running")
}

func TestListAllEnvironments_VMStatusIncludesRepositoryAndBranch(t *testing.T) {
	baseDir := t.TempDir()

	createVMMetadataWithRepo(t, baseDir, "my-vm", "cer", "isolarium", "main")

	stateProvider := func(name, envType string) string {
		return "running"
	}

	envs := ListAllEnvironments(baseDir, stateProvider)

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	env := envs[0]
	if env.Repository != "cer/isolarium" {
		t.Errorf("expected Repository 'cer/isolarium', got %q", env.Repository)
	}
	if env.Branch != "main" {
		t.Errorf("expected Branch 'main', got %q", env.Branch)
	}
}

func TestListAllEnvironments_ContainerStatusIncludesWorkDirectory(t *testing.T) {
	baseDir := t.TempDir()

	createContainerMetadata(t, baseDir, "my-container", "/home/user/repo")

	stateProvider := func(name, envType string) string {
		return "running"
	}

	envs := ListAllEnvironments(baseDir, stateProvider)

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	env := envs[0]
	if env.WorkDirectory != "/home/user/repo" {
		t.Errorf("expected WorkDirectory '/home/user/repo', got %q", env.WorkDirectory)
	}
}

func TestListAllEnvironments_FilterByName(t *testing.T) {
	baseDir := t.TempDir()

	createVMMetadata(t, baseDir, "vm-one")
	createVMMetadata(t, baseDir, "vm-two")

	stateProvider := func(name, envType string) string {
		return "running"
	}

	envs := ListAllEnvironments(baseDir, stateProvider, WithName("vm-one"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs[0].Name != "vm-one" {
		t.Errorf("expected name 'vm-one', got %q", envs[0].Name)
	}
}

func TestListAllEnvironments_FilterByType(t *testing.T) {
	baseDir := t.TempDir()

	createVMMetadata(t, baseDir, "my-vm")
	createContainerMetadata(t, baseDir, "my-container", "/home/user/repo")

	stateProvider := func(name, envType string) string {
		return "running"
	}

	envs := ListAllEnvironments(baseDir, stateProvider, WithType("container"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs[0].Type != "container" {
		t.Errorf("expected type 'container', got %q", envs[0].Type)
	}
}

func TestListAllEnvironments_FilterByNameAndType(t *testing.T) {
	baseDir := t.TempDir()

	createVMMetadata(t, baseDir, "shared-name")
	createContainerMetadata(t, baseDir, "shared-name", "/home/user/repo")

	stateProvider := func(name, envType string) string {
		return "running"
	}

	envs := ListAllEnvironments(baseDir, stateProvider, WithName("shared-name"), WithType("container"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs[0].Name != "shared-name" || envs[0].Type != "container" {
		t.Errorf("expected shared-name/container, got %s/%s", envs[0].Name, envs[0].Type)
	}
}

func TestListAllEnvironments_EmptyWhenNoEnvironments(t *testing.T) {
	baseDir := t.TempDir()

	stateProvider := func(name, envType string) string {
		return "none"
	}

	envs := ListAllEnvironments(baseDir, stateProvider)

	if len(envs) != 0 {
		t.Fatalf("expected 0 environments, got %d", len(envs))
	}
}

func TestListAllEnvironments_UsesStateProviderForEachEnvironment(t *testing.T) {
	baseDir := t.TempDir()

	createVMMetadata(t, baseDir, "running-vm")
	createContainerMetadata(t, baseDir, "stopped-container", "/work")

	stateProvider := func(name, envType string) string {
		if name == "running-vm" {
			return "running"
		}
		return "stopped"
	}

	envs := ListAllEnvironments(baseDir, stateProvider)

	assertContainsEnvironment(t, envs, "running-vm", "vm", "running")
	assertContainsEnvironment(t, envs, "stopped-container", "container", "stopped")
}

// --- helpers ---

func createVMMetadata(t *testing.T, baseDir, name string) {
	t.Helper()
	createVMMetadataWithRepo(t, baseDir, name, "", "", "")
}

func createVMMetadataWithRepo(t *testing.T, baseDir, name, owner, repo, branch string) {
	t.Helper()
	dir := filepath.Join(baseDir, name, "vm")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create vm dir: %v", err)
	}

	metadata := `{"owner":"` + owner + `","repo":"` + repo + `","branch":"` + branch + `"}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("failed to write vm metadata: %v", err)
	}
}

func createContainerMetadata(t *testing.T, baseDir, name, workDir string) {
	t.Helper()
	dir := filepath.Join(baseDir, name, "container")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create container dir: %v", err)
	}

	metadata := `{"type":"container","work_directory":"` + workDir + `"}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("failed to write container metadata: %v", err)
	}
}

func assertContainsEnvironment(t *testing.T, envs []EnvironmentStatus, name, envType, state string) {
	t.Helper()
	for _, env := range envs {
		if env.Name == name && env.Type == envType && env.State == state {
			return
		}
	}
	t.Errorf("expected environment (name=%q, type=%q, state=%q) not found in %+v", name, envType, state, envs)
}
