package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/config"
)

func TestPrepareBuildContextCopiesScriptsAndRegeneratesDockerfile(t *testing.T) {
	projectDir := t.TempDir()
	contextDir := t.TempDir()

	scriptsDir := filepath.Join(projectDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "install-go.sh"), []byte("#!/bin/bash\necho install go"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(baseDockerfile), 0644); err != nil {
		t.Fatal(err)
	}

	scripts := []config.ScriptEntry{
		{Path: "scripts/install-go.sh"},
	}

	err := PrepareBuildContext(contextDir, projectDir, scripts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	copiedScript := filepath.Join(contextDir, "install-go.sh")
	data, err := os.ReadFile(copiedScript)
	if err != nil {
		t.Fatalf("expected script to be copied to build context: %v", err)
	}
	if string(data) != "#!/bin/bash\necho install go" {
		t.Errorf("copied script content mismatch: %s", string(data))
	}

	dockerfile, err := os.ReadFile(filepath.Join(contextDir, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsInOrder(t, string(dockerfile),
		"COPY --chmod=755 install-go.sh /home/isolarium/install-go.sh",
		"RUN /home/isolarium/install-go.sh",
		`CMD ["sleep", "infinity"]`,
	)
}

func TestPrepareBuildContextCopiesMultipleScripts(t *testing.T) {
	projectDir := t.TempDir()
	contextDir := t.TempDir()

	scriptsDir := filepath.Join(projectDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "a.sh"), []byte("script-a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "b.sh"), []byte("script-b"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(baseDockerfile), 0644); err != nil {
		t.Fatal(err)
	}

	scripts := []config.ScriptEntry{
		{Path: "scripts/a.sh"},
		{Path: "scripts/b.sh"},
	}

	err := PrepareBuildContext(contextDir, projectDir, scripts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"a.sh", "b.sh"} {
		if _, err := os.Stat(filepath.Join(contextDir, name)); err != nil {
			t.Errorf("expected %s to exist in build context: %v", name, err)
		}
	}
}

func TestPrepareBuildContextReturnsErrorForMissingScript(t *testing.T) {
	projectDir := t.TempDir()
	contextDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(baseDockerfile), 0644); err != nil {
		t.Fatal(err)
	}

	scripts := []config.ScriptEntry{
		{Path: "scripts/nonexistent.sh"},
	}

	err := PrepareBuildContext(contextDir, projectDir, scripts)
	if err == nil {
		t.Fatal("expected error for missing script file")
	}
	if !strings.Contains(err.Error(), "nonexistent.sh") {
		t.Errorf("expected error to mention missing file, got: %v", err)
	}
}

func TestValidateAndCollectBuildArgsReturnsEnvVarValues(t *testing.T) {
	t.Setenv("MY_TOKEN", "secret123")
	t.Setenv("MY_KEY", "key456")

	scripts := []config.ScriptEntry{
		{Path: "install.sh", Env: []string{"MY_TOKEN", "MY_KEY"}},
	}

	args, err := ValidateAndCollectBuildArgs(scripts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if args["MY_TOKEN"] != "secret123" {
		t.Errorf("expected MY_TOKEN=secret123, got %q", args["MY_TOKEN"])
	}
	if args["MY_KEY"] != "key456" {
		t.Errorf("expected MY_KEY=key456, got %q", args["MY_KEY"])
	}
}

func TestValidateAndCollectBuildArgsReturnsErrorForMissingEnvVar(t *testing.T) {
	scripts := []config.ScriptEntry{
		{Path: "install.sh", Env: []string{"DEFINITELY_NOT_SET_VAR_XYZ"}},
	}

	_, err := ValidateAndCollectBuildArgs(scripts)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "DEFINITELY_NOT_SET_VAR_XYZ") {
		t.Errorf("expected error to list missing variable, got: %v", err)
	}
}

func TestValidateAndCollectBuildArgsReturnsEmptyMapWhenNoEnvVars(t *testing.T) {
	scripts := []config.ScriptEntry{
		{Path: "install.sh"},
	}

	args, err := ValidateAndCollectBuildArgs(scripts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 0 {
		t.Errorf("expected empty map, got %v", args)
	}
}

func TestCreatorPassesBuildArgsToDockerBuild(t *testing.T) {
	metadataDir := t.TempDir()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("docker", "info").Returns("")
	hostUID := fmt.Sprintf("HOST_UID=%d", os.Getuid())
	runner.OnCommand("docker", "build", "-t", "isolarium:latest",
		"--build-arg", hostUID,
		"--build-arg", "MY_TOKEN=secret123",
		metadataDir,
	).Returns("")
	runner.OnCommand("docker", "run", "-d",
		"--name", "my-env",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-v", "/home/user/project:/home/isolarium/repo",
		"-v", knownHostsVolume(),
		"isolarium:latest",
	).Returns("container-id\n")

	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
		ImageTag:    "isolarium:latest",
		BuildArgs:   map[string]string{"MY_TOKEN": "secret123"},
	}

	err := creator.Create("my-env", "/home/user/project", metadataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.VerifyExecuted()
}
