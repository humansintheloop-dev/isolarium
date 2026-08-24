package ec2

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureKeypair_GeneratesKeyWithCorrectModes(t *testing.T) {
	base := t.TempDir()

	publicKey, err := EnsureKeypair(base)
	if err != nil {
		t.Fatalf("EnsureKeypair() error = %v", err)
	}

	assertFileModeAndNotEmpty(t, PrivateKeyPath(base), 0600)
	assertFileModeAndNotEmpty(t, PublicKeyPath(base), 0644)

	if !strings.HasPrefix(publicKey, "ssh-ed25519 ") {
		t.Errorf("EnsureKeypair() returned %q, want it to begin with %q", publicKey, "ssh-ed25519 ")
	}
	if onDisk := readFile(t, PublicKeyPath(base)); !strings.HasPrefix(onDisk, "ssh-ed25519 ") {
		t.Errorf("id_ed25519.pub = %q, want it to begin with %q", onDisk, "ssh-ed25519 ")
	}
	if privateKey := readFile(t, PrivateKeyPath(base)); !strings.HasPrefix(privateKey, "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Errorf("id_ed25519 is not an OpenSSH private key: %q", firstLine(privateKey))
	}
}

func TestEnsureKeypair_ReusesExistingKey(t *testing.T) {
	base := t.TempDir()

	first, err := EnsureKeypair(base)
	if err != nil {
		t.Fatalf("first EnsureKeypair() error = %v", err)
	}
	firstPrivate := readFile(t, PrivateKeyPath(base))
	firstPublic := readFile(t, PublicKeyPath(base))

	second, err := EnsureKeypair(base)
	if err != nil {
		t.Fatalf("second EnsureKeypair() error = %v", err)
	}

	if second != first {
		t.Errorf("second EnsureKeypair() returned %q, want the first key %q", second, first)
	}
	if got := readFile(t, PrivateKeyPath(base)); got != firstPrivate {
		t.Error("second EnsureKeypair() rewrote the private key")
	}
	if got := readFile(t, PublicKeyPath(base)); got != firstPublic {
		t.Error("second EnsureKeypair() rewrote the public key")
	}
}

func TestEnsureKeypair_RegeneratesPublicHalfFromExistingPrivateKey(t *testing.T) {
	base := t.TempDir()
	first, err := EnsureKeypair(base)
	if err != nil {
		t.Fatalf("first EnsureKeypair() error = %v", err)
	}
	if err := os.Remove(PublicKeyPath(base)); err != nil {
		t.Fatalf("removing id_ed25519.pub: %v", err)
	}

	second, err := EnsureKeypair(base)
	if err != nil {
		t.Fatalf("second EnsureKeypair() error = %v", err)
	}

	if second != first {
		t.Errorf("EnsureKeypair() returned %q after losing the public half, want %q", second, first)
	}
	assertFileModeAndNotEmpty(t, PublicKeyPath(base), 0644)
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func firstLine(content string) string {
	if index := strings.Index(content, "\n"); index >= 0 {
		return content[:index]
	}
	return content
}
