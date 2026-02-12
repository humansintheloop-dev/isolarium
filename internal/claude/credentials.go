package claude

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

func defaultRunCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var runCommand = defaultRunCommand

func ReadCredentialsFromKeychain() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	output, err := runCommand("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-a", u.Username, "-w")
	if err != nil {
		return "", fmt.Errorf("failed to read Claude credentials from Keychain: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
