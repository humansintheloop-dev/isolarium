package backend

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEnvironmentTypeFindsContainerWhenOnlyContainerExists(t *testing.T) {
	baseDir := t.TempDir()
	name := "test-env"

	containerDir := filepath.Join(baseDir, name, "container")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("failed to create container dir: %v", err)
	}

	envType, err := ResolveEnvironmentType(baseDir, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envType != "container" {
		t.Errorf("expected %q, got %q", "container", envType)
	}
}

func TestResolveEnvironmentTypeFindsVMWhenOnlyVMExists(t *testing.T) {
	baseDir := t.TempDir()
	name := "test-env"

	vmDir := filepath.Join(baseDir, name, "vm")
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		t.Fatalf("failed to create vm dir: %v", err)
	}

	envType, err := ResolveEnvironmentType(baseDir, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envType != "vm" {
		t.Errorf("expected %q, got %q", "vm", envType)
	}
}

func TestResolveEnvironmentTypeFindsNonoWhenOnlyNonoExists(t *testing.T) {
	baseDir := t.TempDir()
	name := "test-env"

	nonoDir := filepath.Join(baseDir, name, "nono")
	if err := os.MkdirAll(nonoDir, 0755); err != nil {
		t.Fatalf("failed to create nono dir: %v", err)
	}

	envType, err := ResolveEnvironmentType(baseDir, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envType != "nono" {
		t.Errorf("expected %q, got %q", "nono", envType)
	}
}

func TestResolveEnvironmentTypeReturnsErrorWhenBothExist(t *testing.T) {
	baseDir := t.TempDir()
	name := "test-env"

	for _, subdir := range []string{"vm", "container"} {
		dir := filepath.Join(baseDir, name, subdir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create %s dir: %v", subdir, err)
		}
	}

	_, err := ResolveEnvironmentType(baseDir, name)
	if err == nil {
		t.Fatal("expected error when both vm and container exist, got nil")
	}
	if !strings.Contains(err.Error(), "multiple environments found") {
		t.Errorf("expected error containing %q, got %q", "multiple environments found", err.Error())
	}
}

func TestResolveEnvironmentTypeReturnsErrorWhenNoneExist(t *testing.T) {
	baseDir := t.TempDir()
	name := "test-env"

	_, err := ResolveEnvironmentType(baseDir, name)
	if err == nil {
		t.Fatal("expected error when no environment exists, got nil")
	}
	if !strings.Contains(err.Error(), "no environment found") {
		t.Errorf("expected error containing %q, got %q", "no environment found", err.Error())
	}
	if !errors.Is(err, ErrNoEnvironmentFound) {
		t.Errorf("expected error to wrap ErrNoEnvironmentFound, got %v", err)
	}
}
