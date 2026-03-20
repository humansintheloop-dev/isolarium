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

	tmpDir := copyToTempGitRepo(t, springBootDir)

	createVMFromTempRepo(t, binary, tmpDir, projectRoot)
	t.Cleanup(func() { destroyVM(t, binary, tmpDir) })

	gradlewCmd := "source ~/.sdkman/bin/sdkman-init.sh && ./gradlew clean build"
	gradlewArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", gradlewCmd}
	cmd := exec.Command(binary, gradlewArgs...)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}
