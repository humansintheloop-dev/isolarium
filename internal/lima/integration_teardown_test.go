//go:build integration_teardown

package lima

import (
	"testing"
)

func TestDestroyVM_Integration(t *testing.T) {
	ensureVMRunning(t)

	if err := DestroyVM(vmName); err != nil {
		t.Fatalf("DestroyVM failed: %v", err)
	}

	exists, err := VMExists(vmName)
	if err != nil {
		t.Fatalf("failed to check VM status: %v", err)
	}
	if exists {
		t.Error("VM still exists after destroy")
	}
}

func TestDestroyVM_Idempotent_Integration(t *testing.T) {
	if err := DestroyVM(vmName); err != nil {
		t.Fatalf("first DestroyVM with no VM failed: %v", err)
	}

	if err := DestroyVM(vmName); err != nil {
		t.Fatalf("second DestroyVM with no VM failed: %v", err)
	}
}
