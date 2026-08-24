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

func TestEnvironmentTypeAcceptsEC2(t *testing.T) {
	var e environmentType
	err := e.Set("ec2")
	if err != nil {
		t.Fatalf("expected ec2 to be accepted, got error: %v", err)
	}
	if string(e) != "ec2" {
		t.Errorf("expected %q, got %q", "ec2", string(e))
	}
}

func TestEnvironmentTypeRejectsInvalidValueWithEC2InMessage(t *testing.T) {
	var e environmentType
	err := e.Set("invalid")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "ec2") {
		t.Errorf("expected error message to mention ec2, got %q", err.Error())
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
