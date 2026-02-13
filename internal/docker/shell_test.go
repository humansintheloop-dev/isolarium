package docker

import (
	"testing"
)

func TestBuildShellCommand_ProducesInteractiveExecWithWorkDir(t *testing.T) {
	envVars := map[string]string{"GH_TOKEN": "ghs_abc123"}
	cmd := BuildShellCommand("my-container", envVars)
	expected := []string{
		"docker", "exec", "-it",
		"-e", "GH_TOKEN=ghs_abc123",
		"-w", "/home/isolarium/repo",
		"my-container",
		"bash",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildShellCommand_WithoutEnvVars(t *testing.T) {
	cmd := BuildShellCommand("my-container", nil)
	expected := []string{
		"docker", "exec", "-it",
		"-w", "/home/isolarium/repo",
		"my-container",
		"bash",
	}
	assertArgsEqual(t, expected, cmd)
}

func TestBuildShellCommand_WithMultipleEnvVars(t *testing.T) {
	envVars := map[string]string{"GH_TOKEN": "ghs_abc123", "API_KEY": "key456"}
	cmd := BuildShellCommand("my-container", envVars)
	expected := []string{
		"docker", "exec", "-it",
		"-e", "API_KEY=key456",
		"-e", "GH_TOKEN=ghs_abc123",
		"-w", "/home/isolarium/repo",
		"my-container",
		"bash",
	}
	assertArgsEqual(t, expected, cmd)
}
