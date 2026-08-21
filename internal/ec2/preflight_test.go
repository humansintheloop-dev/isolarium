package ec2

import (
	"errors"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

func lookupEnvReturning(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestRequireRegion_ReturnsAWSRegion(t *testing.T) {
	region, err := RequireRegion(lookupEnvReturning(map[string]string{"AWS_REGION": "us-west-2"}))
	if err != nil {
		t.Fatalf("RequireRegion() returned error: %v", err)
	}
	if region != "us-west-2" {
		t.Errorf("RequireRegion() = %q, want %q", region, "us-west-2")
	}
}

const missingRegionMessage = "AWS_REGION is required for --type ec2; set it in .env.local"

func TestRequireRegion_ErrorsWhenUnset(t *testing.T) {
	_, err := RequireRegion(lookupEnvReturning(map[string]string{}))
	if err == nil {
		t.Fatal("RequireRegion() returned nil error when AWS_REGION is unset")
	}
	if err.Error() != missingRegionMessage {
		t.Errorf("RequireRegion() error = %q, want %q", err.Error(), missingRegionMessage)
	}
}

func TestRequireRegion_ErrorsWhenEmpty(t *testing.T) {
	_, err := RequireRegion(lookupEnvReturning(map[string]string{"AWS_REGION": ""}))
	if err == nil {
		t.Fatal("RequireRegion() returned nil error when AWS_REGION is empty")
	}
	if err.Error() != missingRegionMessage {
		t.Errorf("RequireRegion() error = %q, want %q", err.Error(), missingRegionMessage)
	}
}

func terraformReporting(t *testing.T, versionJSON string) *command.FakeRunner {
	t.Helper()

	runner := command.NewFakeRunner(t)
	runner.OnCommand("terraform", "version", "-json").Returns(versionJSON)
	return runner
}

func terraformReportingVersion(t *testing.T, version string) *command.FakeRunner {
	t.Helper()

	return terraformReporting(t, `{"terraform_version":"`+version+`","platform":"darwin_arm64"}`)
}

func TestCheckTerraformVersion_AcceptsSupportedVersions(t *testing.T) {
	for _, version := range []string{"1.10.0", "1.10.5", "1.12.1", "2.0.0"} {
		t.Run(version, func(t *testing.T) {
			if err := CheckTerraformVersion(terraformReportingVersion(t, version)); err != nil {
				t.Errorf("CheckTerraformVersion() with %s returned error: %v", version, err)
			}
		})
	}
}

func TestCheckTerraformVersion_RejectsOlderVersions(t *testing.T) {
	for _, version := range []string{"1.9.8", "0.15.0", "1.2.9"} {
		t.Run(version, func(t *testing.T) {
			err := CheckTerraformVersion(terraformReportingVersion(t, version))
			if err == nil {
				t.Fatalf("CheckTerraformVersion() with %s returned nil error", version)
			}
			want := "terraform 1.10.0 or later is required for --type ec2 (found " + version + ")"
			if err.Error() != want {
				t.Errorf("CheckTerraformVersion() error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestCheckTerraformVersion_AsksTerraformForMachineReadableOutput(t *testing.T) {
	runner := terraformReportingVersion(t, "1.10.5")

	_ = CheckTerraformVersion(runner)

	runner.VerifyExecuted()
	if len(runner.Calls()) != 1 {
		t.Fatalf("CheckTerraformVersion() made %d calls, want 1", len(runner.Calls()))
	}
	if got := strings.Join(runner.Calls()[0], " "); got != "terraform version -json" {
		t.Errorf("CheckTerraformVersion() ran %q, want %q", got, "terraform version -json")
	}
}

func TestCheckTerraformVersion_ErrorsWhenTheOutputIsUnparseable(t *testing.T) {
	err := CheckTerraformVersion(terraformReporting(t, "Terraform v1.10.5\n"))

	if err == nil {
		t.Fatal("CheckTerraformVersion() returned nil error for unparseable output")
	}
	assertMentionsRequirement(t, err)
	if !strings.Contains(err.Error(), "could not read the version") {
		t.Errorf("CheckTerraformVersion() error = %q, want it to say the version could not be read", err.Error())
	}
}

func TestCheckTerraformVersion_ErrorsWhenTheVersionFieldIsMissing(t *testing.T) {
	err := CheckTerraformVersion(terraformReporting(t, `{"platform":"darwin_arm64"}`))

	if err == nil {
		t.Fatal("CheckTerraformVersion() returned nil error when terraform_version is absent")
	}
	assertMentionsRequirement(t, err)
}

func TestCheckTerraformVersion_ErrorsWhenTheBinaryIsMissing(t *testing.T) {
	missing := errors.New(`exec: "terraform": executable file not found in $PATH`)
	runner := command.NewFakeRunner(t)
	runner.OnCommand("terraform", "version", "-json").Fails(missing)

	err := CheckTerraformVersion(runner)

	if err == nil {
		t.Fatal("CheckTerraformVersion() returned nil error when terraform could not be run")
	}
	if !errors.Is(err, missing) {
		t.Errorf("CheckTerraformVersion() error = %v, want it to wrap %v", err, missing)
	}
	assertMentionsRequirement(t, err)
}

func assertMentionsRequirement(t *testing.T, err error) {
	t.Helper()

	if !strings.Contains(err.Error(), "terraform 1.10.0 or later is required for --type ec2") {
		t.Errorf("error = %q, want it to state the 1.10.0 requirement", err.Error())
	}
}
