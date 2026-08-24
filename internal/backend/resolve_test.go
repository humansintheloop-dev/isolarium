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

func TestResolveBackendReturnsNonoBackendForNono(t *testing.T) {
	b, err := ResolveBackend("nono")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := b.(*NonoBackend); !ok {
		t.Errorf("expected *NonoBackend, got %T", b)
	}
}

func TestResolveBackendReturnsEC2BackendForEC2(t *testing.T) {
	b, err := ResolveBackend("ec2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := b.(*EC2Backend); !ok {
		t.Errorf("expected *EC2Backend, got %T", b)
	}
}

func TestResolveBackendWiresTheSSHTransportForEC2(t *testing.T) {
	b, err := ResolveBackend("ec2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	eb := b.(*EC2Backend)
	if eb.ExecFunc == nil {
		t.Error("resolved EC2Backend has no ExecFunc, so Exec would panic")
	}
	if eb.ExecInteractiveFunc == nil {
		t.Error("resolved EC2Backend has no ExecInteractiveFunc, so ExecInteractive would panic")
	}
}

func TestResolveBackendReturnsErrorForUnknownType(t *testing.T) {
	_, err := ResolveBackend("unknown")
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}
