package cli

import (
	"os"
	"testing"
)

func TestParseEnvFlags_VarEqualsValue_UsesLiteralValue(t *testing.T) {
	result := parseEnvFlags([]string{"FOO=bar"})

	if result["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got FOO=%q", result["FOO"])
	}
}

func TestParseEnvFlags_VarOnly_ReadsFromOsEnvironment(t *testing.T) {
	t.Setenv("MY_TEST_VAR", "from-env")

	result := parseEnvFlags([]string{"MY_TEST_VAR"})

	if result["MY_TEST_VAR"] != "from-env" {
		t.Errorf("expected MY_TEST_VAR=from-env, got MY_TEST_VAR=%q", result["MY_TEST_VAR"])
	}
}

func TestParseEnvFlags_MultipleFlags_Accumulate(t *testing.T) {
	t.Setenv("ALPHA", "a-val")

	result := parseEnvFlags([]string{"ALPHA", "BETA=b-val"})

	if result["ALPHA"] != "a-val" {
		t.Errorf("expected ALPHA=a-val, got ALPHA=%q", result["ALPHA"])
	}
	if result["BETA"] != "b-val" {
		t.Errorf("expected BETA=b-val, got BETA=%q", result["BETA"])
	}
}

func TestParseEnvFlags_UnsetVar_ResultsInEmptyString(t *testing.T) {
	_ = os.Unsetenv("TOTALLY_UNSET_VAR")

	result := parseEnvFlags([]string{"TOTALLY_UNSET_VAR"})

	val, exists := result["TOTALLY_UNSET_VAR"]
	if !exists {
		t.Error("expected TOTALLY_UNSET_VAR to exist in result map")
	}
	if val != "" {
		t.Errorf("expected empty string for unset var, got %q", val)
	}
}

func TestParseEnvFlags_ValueContainsEquals(t *testing.T) {
	result := parseEnvFlags([]string{"DB_URL=host=localhost dbname=test"})

	if result["DB_URL"] != "host=localhost dbname=test" {
		t.Errorf("expected value with equals preserved, got DB_URL=%q", result["DB_URL"])
	}
}

func TestRootCmd_HasEnvPersistentFlag(t *testing.T) {
	cmd := newRootCmdWithResolver(nil)

	flag := cmd.PersistentFlags().Lookup("env")
	if flag == nil {
		t.Fatal("expected --env persistent flag to be registered")
	}
	if flag.Value.Type() != "stringSlice" {
		t.Errorf("expected stringSlice type, got %s", flag.Value.Type())
	}
}
