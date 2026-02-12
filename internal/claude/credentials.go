package claude

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/cer/isolarium/internal/command"
)

type KeychainReader struct {
	Runner command.Runner
}

func (k KeychainReader) ReadCredentials() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	output, err := k.Runner.Run("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-a", u.Username, "-w")
	if err != nil {
		return "", fmt.Errorf("failed to read Claude credentials from Keychain: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func ReadCredentialsFromKeychain() (string, error) {
	return KeychainReader{Runner: command.ExecRunner{}}.ReadCredentials()
}
