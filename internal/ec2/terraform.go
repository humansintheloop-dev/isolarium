package ec2

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

const (
	terraformBinary = "terraform"

	// StateKey is the object key holding the single shared state file inside the
	// remote-state bucket.
	StateKey = "isolarium/terraform.tfstate"
)

// TerraformRunner drives the terraform binary against the shared working
// directory at <base>/ec2/terraform.
type TerraformRunner struct {
	runner command.Runner
	base   string
	bucket string
	region string
}

func NewTerraformRunner(runner command.Runner, base, bucket, region string) *TerraformRunner {
	return &TerraformRunner{runner: runner, base: base, bucket: bucket, region: region}
}

func terraformStateDir(base string) string {
	return filepath.Join(TerraformDir(base), ".terraform")
}

// Init configures the S3 backend, and is a no-op once the working directory has
// already been initialised.
func (r *TerraformRunner) Init() error {
	if fileExists(terraformStateDir(r.base)) {
		return nil
	}
	_, err := r.run("init",
		"-backend-config=bucket="+r.bucket,
		"-backend-config=key="+StateKey,
		"-backend-config=region="+r.region,
	)
	return err
}

func (r *TerraformRunner) Apply(vars map[string]string) error {
	_, err := r.run("apply", mutatingFlags(vars)...)
	return err
}

func (r *TerraformRunner) Destroy(vars map[string]string) error {
	_, err := r.run("destroy", mutatingFlags(vars)...)
	return err
}

func (r *TerraformRunner) OutputJSON() ([]byte, error) {
	return r.run("output", "-json")
}

func (r *TerraformRunner) run(subcommand string, flags ...string) ([]byte, error) {
	args := append([]string{"-chdir=" + TerraformDir(r.base), subcommand}, flags...)

	output, err := r.runner.Run(terraformBinary, args...)
	if err != nil {
		return nil, fmt.Errorf("terraform %s failed: %w\n%s", subcommand, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
