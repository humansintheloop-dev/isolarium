package nono

import (
	"testing"
)

func TestPermissionFlagsContainsAllowCwd(t *testing.T) {
	flags := PermissionFlags()

	assertContainsFlag(t, flags, "--allow-cwd")
}


func assertContainsFlag(t *testing.T, slice []string, flag string) {
	t.Helper()
	for _, v := range slice {
		if v == flag {
			return
		}
	}
	t.Errorf("expected flags to contain %s, got %v", flag, slice)
}

func assertContainsSequence(t *testing.T, slice []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(slice)-1; i++ {
		if slice[i] == flag && slice[i+1] == value {
			return
		}
	}
	t.Errorf("expected flags to contain [%s %s], got %v", flag, value, slice)
}
