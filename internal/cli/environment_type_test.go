package cli

import (
	"strings"
	"testing"
)

func TestEnvironmentTypeAcceptsNono(t *testing.T) {
	var e environmentType
	err := e.Set("nono")
	if err != nil {
		t.Fatalf("expected nono to be accepted, got error: %v", err)
	}
	if string(e) != "nono" {
		t.Errorf("expected %q, got %q", "nono", string(e))
	}
}

func TestEnvironmentTypeRejectsInvalidValueWithNonoInMessage(t *testing.T) {
	var e environmentType
	err := e.Set("invalid")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "nono") {
		t.Errorf("expected error message to mention nono, got %q", err.Error())
	}
}
