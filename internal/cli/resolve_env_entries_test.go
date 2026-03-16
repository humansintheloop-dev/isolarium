package cli

import "testing"

func TestResolveEnvEntries(t *testing.T) {
	t.Setenv("HOST_VAR", "host_value")

	entries := []string{
		"HOST_VAR",
		"GRADLE_OPTS=-Dorg.gradle.daemon=false",
		"EXPLICIT=inline_value",
	}
	result := resolveEnvEntries(entries)

	if result["HOST_VAR"] != "host_value" {
		t.Errorf("expected HOST_VAR='host_value', got %q", result["HOST_VAR"])
	}
	if result["GRADLE_OPTS"] != "-Dorg.gradle.daemon=false" {
		t.Errorf("expected GRADLE_OPTS='-Dorg.gradle.daemon=false', got %q", result["GRADLE_OPTS"])
	}
	if result["EXPLICIT"] != "inline_value" {
		t.Errorf("expected EXPLICIT='inline_value', got %q", result["EXPLICIT"])
	}
}
