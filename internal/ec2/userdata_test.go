package ec2

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const ec2UserDataLimit = 16384

func TestRenderUserData_ContainsToolchainAndIsUnderLimit(t *testing.T) {
	rendered := RenderUserData()

	if !strings.HasPrefix(rendered, "#cloud-config\n") {
		t.Errorf("cloud-init.yaml starts with %q, want it to start with %q",
			firstLine(rendered), "#cloud-config")
	}
	assertContainsAll(t, "cloud-init.yaml", rendered,
		"git",
		"curl",
		"wget",
		"ca-certificates",
		"gnupg",
		"lsb-release",
		"unzip",
		"zip",
		"uidmap",
		"dbus-user-session",
		"tmux",
		"kernel.apparmor_restrict_unprivileged_userns=0",
		"deb.nodesource.com",
		"cli.github.com",
		"get.docker.com/rootless",
		"loginctl enable-linger",
		"get.sdkman.io",
		"claude.ai/install.sh",
		"astral.sh/uv",
	)
	if len(rendered) >= ec2UserDataLimit {
		t.Errorf("cloud-init.yaml renders to %d bytes, want fewer than %d",
			len(rendered), ec2UserDataLimit)
	}
}

// cloud-init runs runcmd as root, so an npm global install leaves a binary in a
// tree the ubuntu user cannot write: Claude Code's auto-update then fails at
// every start. The native installer run as ubuntu puts it in ~/.local/bin
// instead, and naming the release channel keeps one run from silently landing on
// a different train than the one before it.
func TestRenderUserData_InstallsClaudeCodeAsTheUserThatRunsIt(t *testing.T) {
	rendered := RenderUserData()

	assertContainsAll(t, "cloud-init.yaml", rendered,
		"runuser -l ubuntu -c 'curl -fsSL https://claude.ai/install.sh | bash -s stable'")
	assertContainsNone(t, "cloud-init.yaml", rendered, "@anthropic-ai/claude-code")
}

func TestRenderUserData_RunsUserLevelStepsAsUbuntu(t *testing.T) {
	rendered := RenderUserData()

	assertContainsAll(t, "cloud-init.yaml", rendered, "runuser -l ubuntu -c")
	assertContainsNone(t, "cloud-init.yaml", rendered, "$USER")
}

// The toolchain has to be reachable from `ssh <host> <command>`, which runs a
// non-interactive shell that returns out of ~/.bashrc before reaching anything
// exported there. PAM reads /etc/environment for those sessions, so that is
// where the user-level installs have to be published.
func TestRenderUserData_PublishesTheUserToolchainToNonInteractiveSessions(t *testing.T) {
	rendered := RenderUserData()

	assertContainsAll(t, "cloud-init.yaml", rendered,
		"/etc/environment",
		"/home/ubuntu/bin",
		"/home/ubuntu/.local/bin",
		"DOCKER_HOST",
	)
}

func TestRenderUserData_LoadsTheKernelModuleRootlessDockerRefusesToInstallWithout(t *testing.T) {
	assertContainsAll(t, "cloud-init.yaml", RenderUserData(), "nf_tables")
}

func TestRenderUserData_IsParseableCloudConfig(t *testing.T) {
	var document map[string]any

	if err := yaml.Unmarshal([]byte(RenderUserData()), &document); err != nil {
		t.Fatalf("cloud-init.yaml is not valid YAML: %v", err)
	}
	for _, key := range []string{"packages", "runcmd"} {
		if _, present := document[key]; !present {
			t.Errorf("cloud-init.yaml has no %q key", key)
		}
	}
}

func TestRenderUserData_RecordsThatItDuplicatesTheLimaTemplate(t *testing.T) {
	rendered := RenderUserData()

	assertContainsAll(t, "cloud-init.yaml", rendered, "internal/lima/template.yaml")
}
