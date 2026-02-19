package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/status"
)

func TestStatusCommand_ListsBothVMsAndContainers(t *testing.T) {
	lister := &stubEnvironmentLister{
		environments: []status.EnvironmentStatus{
			{Name: "my-vm", Type: "vm", State: "running", Repository: "cer/isolarium", Branch: "main"},
			{Name: "my-container", Type: "container", State: "running", WorkDirectory: "/home/user/repo"},
		},
	}

	rootCmd := newRootCmdWithStatusLister(lister)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	assertOutputContains(t, output, "my-vm")
	assertOutputContains(t, output, "vm")
	assertOutputContains(t, output, "running")
	assertOutputContains(t, output, "cer/isolarium")
	assertOutputContains(t, output, "my-container")
	assertOutputContains(t, output, "container")
	assertOutputContains(t, output, "/home/user/repo")
}

func TestStatusCommand_ContainerShowsWorkDirectory(t *testing.T) {
	lister := &stubEnvironmentLister{
		environments: []status.EnvironmentStatus{
			{Name: "dev", Type: "container", State: "running", WorkDirectory: "/projects/myapp"},
		},
	}

	rootCmd := newRootCmdWithStatusLister(lister)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertOutputContains(t, buf.String(), "/projects/myapp")
}

func TestStatusCommand_FilterByName(t *testing.T) {
	lister := &stubEnvironmentLister{
		environments: []status.EnvironmentStatus{
			{Name: "target-vm", Type: "vm", State: "running"},
		},
	}

	rootCmd := newRootCmdWithStatusLister(lister)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--name", "target-vm"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lister.lastNameFilter != "target-vm" {
		t.Errorf("expected name filter 'target-vm', got %q", lister.lastNameFilter)
	}
}

func TestStatusCommand_FilterByType(t *testing.T) {
	lister := &stubEnvironmentLister{
		environments: []status.EnvironmentStatus{
			{Name: "my-container", Type: "container", State: "stopped"},
		},
	}

	rootCmd := newRootCmdWithStatusLister(lister)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--type", "container"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lister.lastTypeFilter != "container" {
		t.Errorf("expected type filter 'container', got %q", lister.lastTypeFilter)
	}
}

func TestStatusCommand_EmptyListShowsNoEnvironments(t *testing.T) {
	lister := &stubEnvironmentLister{
		environments: []status.EnvironmentStatus{},
	}

	rootCmd := newRootCmdWithStatusLister(lister)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertOutputContains(t, buf.String(), "No environments found")
}

func TestStatusCommand_NonoShowsWorkDirectory(t *testing.T) {
	lister := &stubEnvironmentLister{
		environments: []status.EnvironmentStatus{
			{Name: "my-nono", Type: "nono", State: "configured", WorkDirectory: "/Users/dev/project"},
		},
	}

	rootCmd := newRootCmdWithStatusLister(lister)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	assertOutputContains(t, output, "my-nono")
	assertOutputContains(t, output, "nono")
	assertOutputContains(t, output, "configured")
	assertOutputContains(t, output, "/Users/dev/project")
}

func TestFormatDetails_NonoReturnsWorkDirectory(t *testing.T) {
	env := status.EnvironmentStatus{
		Name:          "my-nono",
		Type:          "nono",
		State:         "configured",
		WorkDirectory: "/Users/dev/project",
	}

	details := formatDetails(env)

	if details != "/Users/dev/project" {
		t.Errorf("expected formatDetails to return '/Users/dev/project', got %q", details)
	}
}

// --- helpers ---

type stubEnvironmentLister struct {
	environments   []status.EnvironmentStatus
	lastNameFilter string
	lastTypeFilter string
}

func (s *stubEnvironmentLister) List(nameFilter, typeFilter string) []status.EnvironmentStatus {
	s.lastNameFilter = nameFilter
	s.lastTypeFilter = typeFilter
	return s.environments
}

func assertOutputContains(t *testing.T, output, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Errorf("expected output to contain %q, got:\n%s", expected, output)
	}
}
