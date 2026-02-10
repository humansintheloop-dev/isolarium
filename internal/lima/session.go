package lima

import (
	"fmt"
	"os/exec"
	"strings"
)

// CopyClaudeCredentials writes Claude credentials content into the VM.
// It creates the ~/.claude directory if needed and sets file permissions to 600.
func CopyClaudeCredentials(name, credentials string) error {
	mkdirArgs := BuildCreateClaudeDirCommand(name)
	cmd := exec.Command(mkdirArgs[0], mkdirArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w\noutput: %s", err, output)
	}

	writeArgs := BuildWriteCredentialsCommand(name)
	cmd = exec.Command(writeArgs[0], writeArgs[1:]...)
	cmd.Stdin = strings.NewReader(credentials)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write credentials: %w\noutput: %s", err, output)
	}

	chmodArgs := BuildChmodCredentialsCommand(name)
	cmd = exec.Command(chmodArgs[0], chmodArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set credentials permissions: %w\noutput: %s", err, output)
	}

	return nil
}

func BuildWriteCredentialsCommand(name string) []string {
	return []string{
		"limactl",
		"shell",
		name,
		"--",
		"bash",
		"-c",
		"cat > ~/.claude/.credentials.json",
	}
}

func BuildCreateClaudeDirCommand(name string) []string {
	return []string{
		"limactl",
		"shell",
		name,
		"--",
		"bash",
		"-c",
		"mkdir -p ~/.claude",
	}
}

func BuildChmodCredentialsCommand(name string) []string {
	return []string{
		"limactl",
		"shell",
		name,
		"--",
		"bash",
		"-c",
		"chmod 600 ~/.claude/.credentials.json",
	}
}
