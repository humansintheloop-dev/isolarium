package lima

import (
	"fmt"
	"os/exec"
	"strings"
)

// CopyClaudeCredentials writes Claude credentials content into the VM.
// It creates the ~/.claude directory if needed and sets file permissions to 600.
func CopyClaudeCredentials(credentials string) error {
	mkdirArgs := BuildCreateClaudeDirCommand()
	cmd := exec.Command(mkdirArgs[0], mkdirArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w\noutput: %s", err, output)
	}

	writeArgs := BuildWriteCredentialsCommand()
	cmd = exec.Command(writeArgs[0], writeArgs[1:]...)
	cmd.Stdin = strings.NewReader(credentials)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write credentials: %w\noutput: %s", err, output)
	}

	chmodArgs := BuildChmodCredentialsCommand()
	cmd = exec.Command(chmodArgs[0], chmodArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set credentials permissions: %w\noutput: %s", err, output)
	}

	return nil
}

// BuildWriteCredentialsCommand builds the limactl command to write credentials
// content (piped via stdin) into the VM.
func BuildWriteCredentialsCommand() []string {
	return []string{
		"limactl",
		"shell",
		GetVMName(),
		"--",
		"bash",
		"-c",
		"cat > ~/.claude/.credentials.json",
	}
}

// BuildCreateClaudeDirCommand builds the command to create the ~/.claude directory in the VM.
func BuildCreateClaudeDirCommand() []string {
	return []string{
		"limactl",
		"shell",
		GetVMName(),
		"--",
		"bash",
		"-c",
		"mkdir -p ~/.claude",
	}
}

// BuildChmodCredentialsCommand builds the command to set permissions on the credentials file.
func BuildChmodCredentialsCommand() []string {
	return []string{
		"limactl",
		"shell",
		GetVMName(),
		"--",
		"bash",
		"-c",
		"chmod 600 ~/.claude/.credentials.json",
	}
}
