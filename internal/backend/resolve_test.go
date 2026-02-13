package backend

import (
	"testing"
)

func TestResolveBackendReturnsLimaBackendForVM(t *testing.T) {
	b, err := ResolveBackend("vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := b.(*LimaBackend); !ok {
		t.Errorf("expected *LimaBackend, got %T", b)
	}
}

func TestResolveBackendReturnsDockerBackendForContainer(t *testing.T) {
	b, err := ResolveBackend("container")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := b.(*DockerBackend); !ok {
		t.Errorf("expected *DockerBackend, got %T", b)
	}
}

func TestResolveBackendReturnsErrorForUnknownType(t *testing.T) {
	_, err := ResolveBackend("unknown")
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}
