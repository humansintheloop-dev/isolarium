package docker

import (
	"testing"
)

func TestBuildCreateClaudeDirCommand_ProducesDockerExecMkdir(t *testing.T) {
	cmd := BuildCreateClaudeDirCommand("my-container")
	expected := []string{
		"docker", "exec", "my-container",
		"mkdir", "-p", "/home/isolarium/.claude",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildWriteCredentialsCommand_ProducesDockerExecWithStdinRedirect(t *testing.T) {
	cmd := BuildWriteCredentialsCommand("my-container")
	expected := []string{
		"docker", "exec", "-i", "my-container",
		"bash", "-c", "cat > /home/isolarium/.claude/.credentials.json",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildChmodCredentialsCommand_ProducesDockerExecChmod(t *testing.T) {
	cmd := BuildChmodCredentialsCommand("my-container")
	expected := []string{
		"docker", "exec", "my-container",
		"chmod", "600", "/home/isolarium/.claude/.credentials.json",
	}
	assertArgsEqual(t, expected, cmd)
}
