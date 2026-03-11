//go:build e2e_gradlew

package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGradlewBuildInContainer_EndToEnd(t *testing.T) {
	binary := buildGradlewBinary(t)
	projectRoot := gradlewProjectRoot(t)

	gradlewCmd := "source ~/.sdkman/bin/sdkman-init.sh && cd testdata/spring-boot-app && ./gradlew clean build"
	gradlewArgs := []string{"--type", "container", "run", "--no-gh-token", "--", "bash", "-c", gradlewCmd}
	cmd := exec.Command(binary, gradlewArgs...)
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in container failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}
