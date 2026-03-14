//go:build e2e_gradlew

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGradlewBuildInVM_EndToEnd(t *testing.T) {
	binary := buildGradlewBinary(t)
	projectRoot := gradlewProjectRoot(t)
	springBootDir := filepath.Join(projectRoot, "testdata", "spring-boot-app")

	createVMForGradlew(t, binary, springBootDir)
	t.Cleanup(func() { destroyVMForGradlew(t, binary, springBootDir) })

	gradlewCmd := "source ~/.sdkman/bin/sdkman-init.sh && ./gradlew clean build"
	gradlewArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", gradlewCmd}
	cmd := exec.Command(binary, gradlewArgs...)
	cmd.Dir = springBootDir
	output, err := cmd.CombinedOutput()
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}

func createVMForGradlew(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "vm", "create")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM create output:\n%s", output)
	if err != nil {
		t.Fatalf("VM create failed: %v", err)
	}
}

func destroyVMForGradlew(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "vm", "destroy")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM destroy output:\n%s", output)
	if err != nil {
		t.Logf("VM destroy failed (ignoring): %v", err)
	}
}
