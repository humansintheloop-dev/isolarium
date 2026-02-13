package docker

import (
	"fmt"
	"os/exec"
	"strings"
)

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

func BuildCreateClaudeDirCommand(name string) []string {
	return []string{
		"docker", "exec", name,
		"mkdir", "-p", "/home/isolarium/.claude",
	}
}

func BuildWriteCredentialsCommand(name string) []string {
	return []string{
		"docker", "exec", "-i", name,
		"bash", "-c", "cat > /home/isolarium/.claude/.credentials.json",
	}
}

func BuildChmodCredentialsCommand(name string) []string {
	return []string{
		"docker", "exec", name,
		"chmod", "600", "/home/isolarium/.claude/.credentials.json",
	}
}
