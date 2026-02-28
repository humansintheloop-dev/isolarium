package claude

import (
	"fmt"
	"os/user"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

func TestReadCredentials_ReturnsCredentials(t *testing.T) {
	u, _ := user.Current()
	runner := command.NewFakeRunner(t)
	runner.OnCommand("security", "find-generic-password", "-s", "Claude Code-credentials", "-a", u.Username, "-w").
		Returns("  some-credentials-json\n")

	reader := KeychainReader{Runner: runner}
	creds, err := reader.ReadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds != "some-credentials-json" {
		t.Errorf("expected trimmed credentials, got %q", creds)
	}
	runner.VerifyExecuted()
}

func TestReadCredentials_ReturnsErrorOnCommandFailure(t *testing.T) {
	runner := command.NewFakeRunner(t)
	runner.OnCommand("security").
		Fails(fmt.Errorf("keychain locked"))

	reader := KeychainReader{Runner: runner}
	_, err := reader.ReadCredentials()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	runner.VerifyExecuted()
}
