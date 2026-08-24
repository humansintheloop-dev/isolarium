package ec2

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

func applyVars() map[string]string {
	return map[string]string{
		"ingress_cidr": "203.0.113.7/32",
		"public_key":   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfake isolarium",
		"region":       testRegion,
	}
}

func testTerraformConfig(base string) TerraformConfig {
	return TerraformConfig{Base: base, Bucket: testBucketName, Region: testRegion}
}

func newTestTerraformRunner(t *testing.T, base string) (*TerraformRunner, *command.FakeRunner) {
	t.Helper()

	runner, fake, _ := newNarratingTerraformRunner(t, base, "")
	return runner, fake
}

func newNarratingTerraformRunner(t *testing.T, base, output string) (*TerraformRunner, *command.FakeRunner, *bytes.Buffer) {
	t.Helper()

	fake := command.NewFakeRunner(t)
	fake.OnCommand("terraform").Returns(output)
	narration := &bytes.Buffer{}
	return NewTerraformRunner(fake, testTerraformConfig(base), narration), fake, narration
}

func TestTerraformRunner_Init_PassesTheThreeBackendConfigFlags(t *testing.T) {
	base := t.TempDir()
	runner, fake := newTestTerraformRunner(t, base)

	if err := runner.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	assertSingleCall(t, fake, strings.Join([]string{
		"terraform",
		"-chdir=" + TerraformDir(base),
		"init",
		"-backend-config=bucket=" + testBucketName,
		"-backend-config=key=" + StateKey,
		"-backend-config=region=" + testRegion,
	}, " "))
}

func TestTerraformRunner_Init_SkipsWhenTheWorkingDirectoryIsAlreadyInitialised(t *testing.T) {
	base := t.TempDir()
	runner, fake := newTestTerraformRunner(t, base)
	markAlreadyInitialised(t, base)

	if err := runner.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if len(fake.Calls()) != 0 {
		t.Errorf("Init() ran %v, want no terraform invocation", fake.Calls())
	}
}

func TestTerraformRunner_MutatingCommands_CarryTheSafetyFlagsAndSortedVars(t *testing.T) {
	mutations := map[string]func(*TerraformRunner, map[string]string) error{
		"apply":   (*TerraformRunner).Apply,
		"destroy": (*TerraformRunner).Destroy,
	}

	for subcommand, mutate := range mutations {
		t.Run(subcommand, func(t *testing.T) {
			base := t.TempDir()
			runner, fake := newTestTerraformRunner(t, base)

			if err := mutate(runner, applyVars()); err != nil {
				t.Fatalf("%s() error = %v", subcommand, err)
			}

			assertSingleCall(t, fake, strings.Join([]string{
				"terraform",
				"-chdir=" + TerraformDir(base),
				subcommand,
				"-auto-approve",
				"-input=false",
				"-lock-timeout=120s",
				"-var=ingress_cidr=203.0.113.7/32",
				"-var=public_key=ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfake isolarium",
				"-var=region=" + testRegion,
			}, " "))
		})
	}
}

func TestTerraformRunner_OutputJSON_ReturnsTheRawDocument(t *testing.T) {
	base := t.TempDir()
	fake := command.NewFakeRunner(t)
	fake.OnCommand("terraform").Returns(`{"instance_id_my-work":{"value":"i-05"}}`)
	runner := NewTerraformRunner(fake, testTerraformConfig(base), nil)

	data, err := runner.OutputJSON()

	if err != nil {
		t.Fatalf("OutputJSON() error = %v", err)
	}
	if string(data) != `{"instance_id_my-work":{"value":"i-05"}}` {
		t.Errorf("OutputJSON() = %q, want the raw terraform document", string(data))
	}
	assertSingleCall(t, fake, strings.Join([]string{
		"terraform", "-chdir=" + TerraformDir(base), "output", "-json",
	}, " "))
}

func TestTerraformRunner_Apply_ReportsTheCommandOutputOnFailure(t *testing.T) {
	base := t.TempDir()
	fake := command.NewFakeRunner(t)
	fake.OnCommand("terraform").Fails(errors.New("exit status 1"))
	runner := NewTerraformRunner(fake, testTerraformConfig(base), nil)

	err := runner.Apply(applyVars())

	if err == nil {
		t.Fatal("Apply() returned nil error when terraform failed")
	}
	if !strings.Contains(err.Error(), "terraform apply") {
		t.Errorf("Apply() error = %q, want it to name the failed command", err.Error())
	}
}

func markAlreadyInitialised(t *testing.T, base string) {
	t.Helper()

	if err := os.MkdirAll(terraformStateDir(base), scaffoldingDirMode); err != nil {
		t.Fatalf("creating %s: %v", terraformStateDir(base), err)
	}
}

func assertSingleCall(t *testing.T, fake *command.FakeRunner, want string) {
	t.Helper()

	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("recorded %d invocations, want exactly 1: %v", len(calls), calls)
	}
	if got := strings.Join(calls[0], " "); got != want {
		t.Errorf("invocation =\n  %s\nwant\n  %s", got, want)
	}
}

func TestTerraformRunner_LongRunningCommands_NarrateThemselvesWhileTheyRun(t *testing.T) {
	commands := map[string]func(*TerraformRunner) error{
		"init":    func(r *TerraformRunner) error { return r.Init() },
		"apply":   func(r *TerraformRunner) error { return r.Apply(applyVars()) },
		"destroy": func(r *TerraformRunner) error { return r.Destroy(applyVars()) },
	}

	for subcommand, invoke := range commands {
		t.Run(subcommand, func(t *testing.T) {
			progress := "aws_vpc.isolarium: Still destroying... [30s elapsed]"
			runner, _, narration := newNarratingTerraformRunner(t, t.TempDir(), progress)

			if err := invoke(runner); err != nil {
				t.Fatalf("%s() error = %v", subcommand, err)
			}

			if narration.String() != progress {
				t.Errorf("%s() narrated %q, want %q", subcommand, narration.String(), progress)
			}
		})
	}
}

// OutputJSON is parsed rather than read, so echoing it would bury the narration
// that matters under a document nobody is meant to read.
func TestTerraformRunner_OutputJSON_StaysSilent(t *testing.T) {
	runner, _, narration := newNarratingTerraformRunner(t, t.TempDir(), `{"instance_id_my-work":{"value":"i-05"}}`)

	if _, err := runner.OutputJSON(); err != nil {
		t.Fatalf("OutputJSON() error = %v", err)
	}

	if narration.Len() != 0 {
		t.Errorf("OutputJSON() narrated %q, want nothing", narration.String())
	}
}

func TestTerraformRunner_ToleratesNoNarrationWriter(t *testing.T) {
	fake := command.NewFakeRunner(t)
	fake.OnCommand("terraform").Returns("")

	if err := NewTerraformRunner(fake, testTerraformConfig(t.TempDir()), nil).Apply(applyVars()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}
