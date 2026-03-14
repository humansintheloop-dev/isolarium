//go:build e2e_gradlew

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const gradlewTestContainerName = "isolarium-gradlew-test"

func TestGradlewBuildInContainer_EndToEnd(t *testing.T) {
	binary := buildGradlewBinary(t)
	projectRoot := gradlewProjectRoot(t)
	springBootDir := filepath.Join(projectRoot, "testdata", "spring-boot-app")

	createContainerForGradlew(t, binary, springBootDir)
	t.Cleanup(func() { destroyContainerForGradlew(t, binary, springBootDir) })

	gradlewCmd := "source ~/.sdkman/bin/sdkman-init.sh && ./gradlew clean build"
	gradlewArgs := []string{"--type", "container", "--name", gradlewTestContainerName, "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", gradlewCmd}
	cmd := exec.Command(binary, gradlewArgs...)
	cmd.Dir = springBootDir
	output, err := cmd.CombinedOutput()
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in container failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}

func createContainerForGradlew(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "container", "--name", gradlewTestContainerName, "create")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("container create output:\n%s", output)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
}

func destroyContainerForGradlew(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "container", "--name", gradlewTestContainerName, "destroy")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("container destroy output:\n%s", output)
	if err != nil {
		t.Logf("container destroy failed (ignoring): %v", err)
	}
}
