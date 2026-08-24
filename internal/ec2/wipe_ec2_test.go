//go:build ec2

// This file holds the half of the wipe outcome that needs the shared instance
// alive, so its name is load-bearing in the same way the z_ and zz_ prefixes
// elsewhere are: it sorts ahead of z_status_ec2_test.go, which is where the
// shared instance is destroyed. The wipe that actually tears the shared
// infrastructure down lives in zzz_wipe_ec2_test.go, which sorts last of all.
package ec2_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// TestEC2Wipe_RefusesWithLiveInstance proves the refusal is a guard on the
// account rather than a message: everything the environments share is still
// there after the command has declined to run.
func TestEC2Wipe_RefusesWithLiveInstance(t *testing.T) {
	environment := sharedInstance(t)

	output := environment.runIsolariumEC2WipeExpectingRefusal()

	environment.assertNamesTheEnvironmentToDestroy(output)
	environment.assertSharedInfrastructureExists()
}

func (e *ec2Environment) runIsolariumEC2WipeExpectingRefusal() string {
	e.t.Helper()

	output, err := e.runIsolariumEC2Wipe()
	if err == nil {
		e.t.Fatalf("isolarium ec2 wipe exited 0 while %s still exists\n%s", e.name, output)
	}
	return output
}

func (e *ec2Environment) assertNamesTheEnvironmentToDestroy(output string) {
	e.t.Helper()

	guidance := fmt.Sprintf("run isolarium destroy --type ec2 --name %s", e.name)
	if !strings.Contains(output, guidance) {
		e.t.Errorf("isolarium ec2 wipe output does not contain %q\n%s", guidance, output)
	}
}

func (e *ec2Environment) runIsolariumEC2Wipe() (string, error) {
	e.t.Helper()

	wipe := exec.Command(isolariumBinary(e.t), "ec2", "wipe")
	wipe.Env = binaryEnvironment()
	return runBinaryStreaming(wipe)
}
