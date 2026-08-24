package ec2

import (
	"fmt"
	"io"
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

// TerraformConfig is where terraform runs and where it keeps the state every EC2
// environment shares. The three travel together through every invocation, so
// they are named once rather than threaded separately.
type TerraformConfig struct {
	Base   string
	Bucket string
	Region string
}

// TerraformRunner drives the terraform binary against the shared working
// directory at <base>/ec2/terraform.
type TerraformRunner struct {
	runner command.Runner
	config TerraformConfig
	// narration receives the output of the commands slow enough that silence
	// would be indistinguishable from a hang. A nil narration keeps them quiet.
	narration io.Writer
}

func NewTerraformRunner(runner command.Runner, config TerraformConfig, narration io.Writer) *TerraformRunner {
	return &TerraformRunner{runner: runner, config: config, narration: narration}
}

func terraformStateDir(base string) string {
	return filepath.Join(TerraformDir(base), ".terraform")
}

// Init configures the S3 backend, and is a no-op once the working directory has
// already been initialised.
func (r *TerraformRunner) Init() error {
	if fileExists(terraformStateDir(r.config.Base)) {
		return nil
	}
	_, err := r.invoke(r.narration, "init",
		"-backend-config=bucket="+r.config.Bucket,
		"-backend-config=key="+StateKey,
		"-backend-config=region="+r.config.Region,
	)
	return err
}

func (r *TerraformRunner) Apply(vars map[string]string) error {
	_, err := r.invoke(r.narration, "apply", mutatingFlags(vars)...)
	return err
}

func (r *TerraformRunner) Destroy(vars map[string]string) error {
	_, err := r.invoke(r.narration, "destroy", mutatingFlags(vars)...)
	return err
}

// OutputJSON passes no narration, because the document it returns is parsed
// rather than read, and echoing it would bury the progress that is worth seeing.
func (r *TerraformRunner) OutputJSON() ([]byte, error) {
	return r.invoke(nil, "output", "-json")
}

// invoke runs one subcommand, copying its output to narration as it arrives so
// that a long retry loop reads as work rather than as a hang.
func (r *TerraformRunner) invoke(narration io.Writer, subcommand string, flags ...string) ([]byte, error) {
	args := append([]string{"-chdir=" + TerraformDir(r.config.Base), subcommand}, flags...)

	output, err := r.runner.RunStreaming(narration, terraformBinary, args...)
	if err != nil {
		return nil, fmt.Errorf("terraform %s failed: %w\n%s", subcommand, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
