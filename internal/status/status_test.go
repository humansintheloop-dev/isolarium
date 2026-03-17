package status

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/project"
)

func TestGetStatus_ReturnsValidVMState(t *testing.T) {
	s := GetStatus("isolarium")

	validStates := map[string]bool{"none": true, "running": true, "stopped": true}
	if !validStates[s.VMState] {
		t.Errorf("expected VMState to be 'none', 'running', or 'stopped', got '%s'", s.VMState)
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenNoCredentials(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "")

	s := GetStatus("isolarium")

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false")
	}
}

func TestGetStatus_ReturnsConfiguredWhenBothEnvVarsSet(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "test-private-key")

	s := GetStatus("isolarium")

	if !s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be true when both env vars are set")
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenOnlyAppIDSet(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "")

	s := GetStatus("isolarium")

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false when only GITHUB_APP_ID is set")
	}
}

func TestGetStatus_ReturnsNotConfiguredWhenOnlyPrivateKeySet(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "test-private-key")

	s := GetStatus("isolarium")

	if s.GitHubAppConfigured {
		t.Error("expected GitHubAppConfigured to be false when only GITHUB_APP_PRIVATE_KEY_PATH is set")
	}
}

func TestStatus_HasRepositoryFields(t *testing.T) {
	expectedRepo := project.GitHubOrgRepo
	s := Status{
		VMState:             "running",
		GitHubAppConfigured: true,
		Repository:          expectedRepo,
		Branch:              "main",
	}

	if s.Repository != expectedRepo {
		t.Errorf("expected Repository %q, got %q", expectedRepo, s.Repository)
	}
	if s.Branch != "main" {
		t.Errorf("expected Branch 'main', got '%s'", s.Branch)
	}
}

func TestListAllEnvironments_ReturnsBothVMAndContainer(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVM("my-vm")
	s.createContainer("my-container", "/home/user/repo")

	envs := ListAllEnvironments(s.baseDir, alwaysState("running"))

	if len(envs) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(envs))
	}

	assertContainsEnvironment(t, envs, expectedEnv{"my-vm", "vm", "running"})
	assertContainsEnvironment(t, envs, expectedEnv{"my-container", "container", "running"})
}

func TestListAllEnvironments_VMStatusIncludesRepositoryAndBranch(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVMWithRepo("my-vm", project.GitHubOrg, project.GitHubRepo, "main")

	envs := ListAllEnvironments(s.baseDir, alwaysState("running"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	env := envs[0]
	expectedRepo := project.GitHubOrgRepo
	if env.Repository != expectedRepo {
		t.Errorf("expected Repository %q, got %q", expectedRepo, env.Repository)
	}
	if env.Branch != "main" {
		t.Errorf("expected Branch 'main', got %q", env.Branch)
	}
}

func TestListAllEnvironments_ContainerStatusIncludesWorkDirectory(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createContainer("my-container", "/home/user/repo")

	envs := ListAllEnvironments(s.baseDir, alwaysState("running"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	env := envs[0]
	if env.WorkDirectory != "/home/user/repo" {
		t.Errorf("expected WorkDirectory '/home/user/repo', got %q", env.WorkDirectory)
	}
}

func TestListAllEnvironments_FilterByName(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVM("vm-one")
	s.createVM("vm-two")

	envs := ListAllEnvironments(s.baseDir, alwaysState("running"), WithName("vm-one"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs[0].Name != "vm-one" {
		t.Errorf("expected name 'vm-one', got %q", envs[0].Name)
	}
}

func TestListAllEnvironments_FilterByType(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVM("my-vm")
	s.createContainer("my-container", "/home/user/repo")

	envs := ListAllEnvironments(s.baseDir, alwaysState("running"), WithType("container"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs[0].Type != "container" {
		t.Errorf("expected type 'container', got %q", envs[0].Type)
	}
}

func TestListAllEnvironments_FilterByNameAndType(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVM("shared-name")
	s.createContainer("shared-name", "/home/user/repo")

	envs := ListAllEnvironments(s.baseDir, alwaysState("running"), WithName("shared-name"), WithType("container"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	assertContainsEnvironment(t, envs, expectedEnv{"shared-name", "container", "running"})
}

func TestListAllEnvironments_EmptyWhenNoEnvironments(t *testing.T) {
	s := newTestEnvSetup(t)

	envs := ListAllEnvironments(s.baseDir, alwaysState("none"))

	if len(envs) != 0 {
		t.Fatalf("expected 0 environments, got %d", len(envs))
	}
}

func TestListAllEnvironments_UsesStateProviderForEachEnvironment(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVM("running-vm")
	s.createContainer("stopped-container", "/work")

	envs := ListAllEnvironments(s.baseDir, func(name, envType string) string {
		if name == "running-vm" {
			return "running"
		}
		return "stopped"
	})

	assertContainsEnvironment(t, envs, expectedEnv{"running-vm", "vm", "running"})
	assertContainsEnvironment(t, envs, expectedEnv{"stopped-container", "container", "stopped"})
}

func TestListAllEnvironments_NonoAppearsWithConfiguredState(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createNono("my-nono", "/Users/dev/project")

	envs := ListAllEnvironments(s.baseDir, alwaysState("configured"))

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	env := envs[0]
	assertContainsEnvironment(t, envs, expectedEnv{"my-nono", "nono", "configured"})
	if env.WorkDirectory != "/Users/dev/project" {
		t.Errorf("expected WorkDirectory '/Users/dev/project', got %q", env.WorkDirectory)
	}
}

func TestListAllEnvironments_NonoAppearsAlongsideVMAndContainer(t *testing.T) {
	s := newTestEnvSetup(t)
	s.createVM("my-vm")
	s.createContainer("my-container", "/home/user/repo")
	s.createNono("my-nono", "/Users/dev/project")

	envs := ListAllEnvironments(s.baseDir, alwaysState("configured"))

	if len(envs) != 3 {
		t.Fatalf("expected 3 environments, got %d", len(envs))
	}

	assertContainsEnvironment(t, envs, expectedEnv{"my-vm", "vm", "configured"})
	assertContainsEnvironment(t, envs, expectedEnv{"my-container", "container", "configured"})
	assertContainsEnvironment(t, envs, expectedEnv{"my-nono", "nono", "configured"})
}

// --- helpers ---

type testEnvSetup struct {
	t       *testing.T
	baseDir string
}

func newTestEnvSetup(t *testing.T) testEnvSetup {
	t.Helper()
	return testEnvSetup{t: t, baseDir: t.TempDir()}
}

func (s testEnvSetup) writeMetadata(name, envType, json string) {
	s.t.Helper()
	dir := filepath.Join(s.baseDir, name, envType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.t.Fatalf("failed to create %s dir: %v", envType, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(json), 0644); err != nil {
		s.t.Fatalf("failed to write %s metadata: %v", envType, err)
	}
}

func (s testEnvSetup) createVM(name string) {
	s.t.Helper()
	s.writeMetadata(name, "vm", `{"owner":"","repo":"","branch":""}`)
}

func (s testEnvSetup) createVMWithRepo(name, owner, repo, branch string) {
	s.t.Helper()
	s.writeMetadata(name, "vm", `{"owner":"`+owner+`","repo":"`+repo+`","branch":"`+branch+`"}`)
}

func (s testEnvSetup) createContainer(name, workDir string) {
	s.t.Helper()
	s.writeMetadata(name, "container", `{"type":"container","work_directory":"`+workDir+`"}`)
}

func (s testEnvSetup) createNono(name, workDir string) {
	s.t.Helper()
	s.writeMetadata(name, "nono", `{"type":"nono","work_directory":"`+workDir+`"}`)
}

func alwaysState(state string) func(string, string) string {
	return func(name, envType string) string { return state }
}

type expectedEnv struct {
	name, envType, state string
}

func (e expectedEnv) matches(env EnvironmentStatus) bool {
	return env.Name == e.name && env.Type == e.envType && env.State == e.state
}

func assertContainsEnvironment(t *testing.T, envs []EnvironmentStatus, expected expectedEnv) {
	t.Helper()
	for _, env := range envs {
		if expected.matches(env) {
			return
		}
	}
	t.Errorf("expected environment %+v not found in %+v", expected, envs)
}
