package claude

import (
	"fmt"
	"os/user"
	"testing"
)

func TestReadCredentialsFromKeychain_ReturnsCredentials(t *testing.T) {
	u, _ := user.Current()
	runCommand = func(name string, args ...string) ([]byte, error) {
		expectedArgs := []string{"find-generic-password", "-s", "Claude Code-credentials", "-a", u.Username, "-w"}
		if name != "security" {
			t.Errorf("expected command 'security', got %q", name)
		}
		for i, arg := range expectedArgs {
			if i >= len(args) || args[i] != arg {
				t.Errorf("expected arg[%d] = %q, got %q", i, arg, args[i])
			}
		}
		return []byte("  some-credentials-json\n"), nil
	}
	defer func() { runCommand = defaultRunCommand }()

	creds, err := ReadCredentialsFromKeychain()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds != "some-credentials-json" {
		t.Errorf("expected trimmed credentials, got %q", creds)
	}
}

func TestReadCredentialsFromKeychain_ReturnsErrorOnCommandFailure(t *testing.T) {
	runCommand = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("keychain locked")
	}
	defer func() { runCommand = defaultRunCommand }()

	_, err := ReadCredentialsFromKeychain()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
