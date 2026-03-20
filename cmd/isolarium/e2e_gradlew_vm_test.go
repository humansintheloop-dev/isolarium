//go:build e2e_gradlew

package main

import (
	"strings"
	"testing"
)

func TestGradlewBuildInVM_EndToEnd(t *testing.T) {
	binary := buildGradlewBinary(t)
	projectRoot := gradlewProjectRoot(t)

	repo := copyToTempGitRepo(t, projectRoot+"/testdata/spring-boot-app")

	createVMFromTempRepo(t, binary, repo, projectRoot)
	t.Cleanup(func() { destroyVM(t, binary, repo) })

	output, err := runInTempVM(t, binary, repo, "bash", "-c", "source ~/.sdkman/bin/sdkman-init.sh && ./gradlew clean build")
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}
