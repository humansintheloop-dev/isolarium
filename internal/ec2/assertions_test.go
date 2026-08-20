package ec2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertDirMode(t *testing.T, dir string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
	if info.Mode().Perm() != want {
		t.Errorf("%s mode = %04o, want %04o", dir, info.Mode().Perm(), want)
	}
}

func assertFileModeAndNotEmpty(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm() != want {
		t.Errorf("%s mode = %04o, want %04o", path, info.Mode().Perm(), want)
	}
	if info.Size() == 0 {
		t.Errorf("%s is empty", path)
	}
}

func readScaffoldingFile(t *testing.T, base, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(TerraformDir(base), name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(data)
}

func assertContainsAll(t *testing.T, name, content string, wanted ...string) {
	t.Helper()

	for _, want := range wanted {
		if !strings.Contains(content, want) {
			t.Errorf("%s does not contain %q", name, want)
		}
	}
}

func assertContainsNone(t *testing.T, name, content string, unwanted ...string) {
	t.Helper()

	for _, forbidden := range unwanted {
		if strings.Contains(content, forbidden) {
			t.Errorf("%s contains %q, want it absent", name, forbidden)
		}
	}
}

func occurrences(content, substring string) int {
	return strings.Count(content, substring)
}
