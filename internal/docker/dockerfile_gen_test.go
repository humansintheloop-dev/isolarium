package docker

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

const baseDockerfile = `FROM ubuntu:24.04
RUN apt-get update
USER isolarium
WORKDIR /home/isolarium
RUN mkdir -p /home/isolarium/repo /home/isolarium/main-repo
WORKDIR /home/isolarium/repo
CMD ["sleep", "infinity"]
`

func TestGenerateDockerfileWithNoScriptsReturnsBaseUnchanged(t *testing.T) {
	result := GenerateDockerfile(baseDockerfile, nil)

	if result != baseDockerfile {
		t.Errorf("expected base Dockerfile unchanged, got:\n%s", result)
	}
}

func TestGenerateDockerfileWithOneScriptAppendsCopyAndRun(t *testing.T) {
	scripts := []config.ScriptEntry{
		{Path: "scripts/install-go.sh"},
	}

	result := GenerateDockerfile(baseDockerfile, scripts)

	assertContainsInOrder(t, result,
		"WORKDIR /home/isolarium/repo",
		"COPY install-go.sh /tmp/install-go.sh",
		`RUN chmod +x /tmp/install-go.sh && /tmp/install-go.sh`,
		`CMD ["sleep", "infinity"]`,
	)
}

func TestGenerateDockerfileWithMultipleScriptsAppendsInOrder(t *testing.T) {
	scripts := []config.ScriptEntry{
		{Path: "scripts/install-go.sh"},
		{Path: "scripts/install-linters.sh"},
		{Path: "scripts/install-codescene.sh"},
	}

	result := GenerateDockerfile(baseDockerfile, scripts)

	assertContainsInOrder(t, result,
		"WORKDIR /home/isolarium/repo",
		"COPY install-go.sh /tmp/install-go.sh",
		"RUN chmod +x /tmp/install-go.sh && /tmp/install-go.sh",
		"COPY install-linters.sh /tmp/install-linters.sh",
		"RUN chmod +x /tmp/install-linters.sh && /tmp/install-linters.sh",
		"COPY install-codescene.sh /tmp/install-codescene.sh",
		"RUN chmod +x /tmp/install-codescene.sh && /tmp/install-codescene.sh",
		`CMD ["sleep", "infinity"]`,
	)
}

func TestGenerateDockerfileWithEnvVarsIncludesArgDeclarations(t *testing.T) {
	scripts := []config.ScriptEntry{
		{
			Path: "scripts/install-codescene.sh",
			Env:  []string{"CS_ACCESS_TOKEN", "CS_ACE_ACCESS_TOKEN"},
		},
	}

	result := GenerateDockerfile(baseDockerfile, scripts)

	assertContainsInOrder(t, result,
		"WORKDIR /home/isolarium/repo",
		"ARG CS_ACCESS_TOKEN",
		"ARG CS_ACE_ACCESS_TOKEN",
		"COPY install-codescene.sh /tmp/install-codescene.sh",
		"RUN chmod +x /tmp/install-codescene.sh && /tmp/install-codescene.sh",
		`CMD ["sleep", "infinity"]`,
	)
}

func assertContainsInOrder(t *testing.T, text string, fragments ...string) {
	t.Helper()
	pos := 0
	for _, frag := range fragments {
		idx := strings.Index(text[pos:], frag)
		if idx < 0 {
			t.Errorf("expected %q after position %d in:\n%s", frag, pos, text)
			return
		}
		pos += idx + len(frag)
	}
}
