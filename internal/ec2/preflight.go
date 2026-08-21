package ec2

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

const regionEnvVar = "AWS_REGION"

// defaultEnvFile names where the caller is expected to keep AWS credentials, so
// the failure tells them the file to edit rather than just the variable.
const defaultEnvFile = ".env.local"

func RequireRegion(lookupEnv func(string) (string, bool)) (string, error) {
	region, ok := lookupEnv(regionEnvVar)
	if !ok || region == "" {
		return "", fmt.Errorf("%s is required for --type ec2; set it in %s", regionEnvVar, defaultEnvFile)
	}
	return region, nil
}

// minimumTerraform is the oldest release whose S3 backend supports the native
// state locking the generated backend.tf asks for.
var minimumTerraform = terraformVersion{major: 1, minor: 10, patch: 0}

// terraformRequirement opens every version failure, because whatever went wrong
// the caller's next move is the same: install a newer terraform.
var terraformRequirement = fmt.Sprintf("terraform %s or later is required for --type ec2", minimumTerraform)

func CheckTerraformVersion(runner command.Runner) error {
	output, err := runner.Run("terraform", "version", "-json")
	if err != nil {
		return fmt.Errorf("%s, but running it failed: %w", terraformRequirement, err)
	}

	found, err := parseTerraformVersion(output)
	if err != nil {
		return fmt.Errorf("%s, but %w", terraformRequirement, err)
	}
	if found.olderThan(minimumTerraform) {
		return fmt.Errorf("%s (found %s)", terraformRequirement, found)
	}
	return nil
}

type terraformVersion struct {
	major int
	minor int
	patch int
}

func (v terraformVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

// olderThan compares the fields numerically, because the release ordering and
// the lexical ordering disagree: 1.9.8 sorts after 1.10.0 as text.
func (v terraformVersion) olderThan(other terraformVersion) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	return v.patch < other.patch
}

func parseTerraformVersion(output []byte) (terraformVersion, error) {
	var report struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(output, &report); err != nil {
		return terraformVersion{}, fmt.Errorf("could not read the version it reported: %v", err)
	}
	if report.TerraformVersion == "" {
		return terraformVersion{}, fmt.Errorf("could not read the version it reported: no terraform_version field")
	}
	return parseVersionNumber(report.TerraformVersion)
}

// parseVersionNumber reads the leading major.minor.patch and ignores any
// pre-release suffix, so a 1.11.0-beta1 host is judged on its release number.
func parseVersionNumber(version string) (terraformVersion, error) {
	fields := strings.SplitN(strings.SplitN(version, "-", 2)[0], ".", 4)
	if len(fields) < 3 {
		return terraformVersion{}, fmt.Errorf("could not read the version it reported: %q is not major.minor.patch", version)
	}

	var parsed terraformVersion
	for _, field := range []struct {
		text string
		into *int
	}{{fields[0], &parsed.major}, {fields[1], &parsed.minor}, {fields[2], &parsed.patch}} {
		number, err := strconv.Atoi(field.text)
		if err != nil {
			return terraformVersion{}, fmt.Errorf("could not read the version it reported: %q is not major.minor.patch", version)
		}
		*field.into = number
	}
	return parsed, nil
}
