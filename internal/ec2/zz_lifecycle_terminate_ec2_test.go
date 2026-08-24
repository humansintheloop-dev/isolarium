//go:build ec2

// The zz_ prefix is load-bearing. go test walks a package's test files in
// sorted-filename order, and this file has to be the last one, because the test
// it holds destroys the instance every other ec2 test shares.
package ec2_test

import (
	"context"
	"testing"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// TestEC2Lifecycle_Terminates ends the suite by tearing down the shared
// instance and asking AWS directly what became of it, so that "destroy
// returned" is never mistaken for "the instance is gone".
func TestEC2Lifecycle_Terminates(t *testing.T) {
	environment := sharedInstance(t)

	environment.destroy()
	environment.assertInstanceIsTerminated()
}

func (e *ec2Environment) destroy() {
	e.t.Helper()

	if e.destroyed {
		return
	}
	if err := e.backend.Destroy(e.name); err != nil {
		e.t.Fatalf("destroying %s: %v", e.name, err)
	}
	e.destroyed = true
}

func (e *ec2Environment) assertInstanceIsTerminated() {
	e.t.Helper()

	described, err := e.ec2API().DescribeInstances(context.Background(), &awsec2.DescribeInstancesInput{
		InstanceIds: []string{e.instanceID},
	})
	if err != nil {
		e.t.Fatalf("describing instance %s: %v", e.instanceID, err)
	}

	state := reportedInstanceState(described)
	if state != string(awsec2types.InstanceStateNameTerminated) && state != string(awsec2types.InstanceStateNameShuttingDown) {
		e.t.Errorf("instance %s state = %q, want terminated or shutting-down", e.instanceID, state)
	}
}

func reportedInstanceState(described *awsec2.DescribeInstancesOutput) string {
	for _, reservation := range described.Reservations {
		for _, instance := range reservation.Instances {
			if instance.State != nil {
				return string(instance.State.Name)
			}
		}
	}
	return ""
}
