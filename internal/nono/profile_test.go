package nono

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedProfileIsValidJSON(t *testing.T) {
	if !json.Valid(embeddedProfile) {
		t.Fatal("embedded profile is not valid JSON")
	}
}

func TestGetProfilePathReturnsExistingFile(t *testing.T) {
	p := getProfilePath()

	if _, err := os.Stat(p); err != nil {
		t.Fatalf("profile file does not exist at %s: %v", p, err)
	}
}

func TestGetProfilePathReturnsSamePathOnRepeatedCalls(t *testing.T) {
	p1 := getProfilePath()
	p2 := getProfilePath()

	if p1 != p2 {
		t.Errorf("expected same path, got %s and %s", p1, p2)
	}
}

func TestProfileFileMatchesEmbeddedContent(t *testing.T) {
	content, err := os.ReadFile(getProfilePath())
	if err != nil {
		t.Fatalf("failed to read profile file: %v", err)
	}

	if string(content) != string(embeddedProfile) {
		t.Error("profile file content does not match embedded profile")
	}
}

func TestProfileFileIsReadOnly(t *testing.T) {
	info, err := os.Stat(getProfilePath())
	if err != nil {
		t.Fatalf("failed to stat profile file: %v", err)
	}

	if info.Mode().Perm() != 0400 {
		t.Errorf("expected file permissions 0400, got %04o", info.Mode().Perm())
	}
}

func TestProfileDirIsOwnerOnly(t *testing.T) {
	dir := filepath.Dir(getProfilePath())
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat profile dir: %v", err)
	}

	if info.Mode().Perm() != fs.FileMode(0700) {
		t.Errorf("expected dir permissions 0700, got %04o", info.Mode().Perm())
	}
}
