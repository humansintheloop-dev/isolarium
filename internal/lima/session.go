package lima

import (
	"fmt"
	"os/exec"
)

// CopyClaudeCredentials copies the Claude credentials file from the host to the VM.
// It creates the ~/.claude directory if needed and sets file permissions to 600.
func CopyClaudeCredentials(credentialsPath string) error {
	// Create the ~/.claude directory in the VM
	mkdirArgs := BuildCreateClaudeDirCommand()
	cmd := exec.Command(mkdirArgs[0], mkdirArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w\noutput: %s", err, output)
	}

	// Copy the credentials file
	copyArgs := BuildCopyCredentialsCommand(credentialsPath)
	cmd = exec.Command(copyArgs[0], copyArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy credentials: %w\ncommand: %v\noutput: %s", err, copyArgs, output)
	}

	// Set permissions to 600
	chmodArgs := BuildChmodCredentialsCommand()
	cmd = exec.Command(chmodArgs[0], chmodArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set credentials permissions: %w\noutput: %s", err, output)
	}

	return nil
}

// BuildCopyCredentialsCommand builds the limactl command to copy Claude credentials to the VM.
// Uses $HOME instead of ~ because tilde expansion doesn't work in command arguments.
func BuildCopyCredentialsCommand(credentialsPath string) []string {
	return []string{
		"limactl",
		"copy",
		credentialsPath,
		GetVMName() + ":" + credentialsDestPath(),
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

// credentialsDestPath returns the destination path for credentials in the VM.
// Uses .claude/ relative path which limactl resolves to the user's home directory.
func credentialsDestPath() string {
	return ".claude/.credentials.json"
}
