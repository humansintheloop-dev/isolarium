//go:build ec2

// The zzz_ prefix is load-bearing. go test walks a package's test files in
// sorted-filename order, and this file has to be the last one of all — after
// even zz_lifecycle_terminate_ec2_test.go — because the wipe it performs
// removes the VPC, security group and key pair every other ec2 test depends on.
package ec2_test

import "testing"

// TestEC2Wipe_RemovesInfraAndRetainsBucket asks AWS what became of the shared
// infrastructure, because removing the local Terraform working directory would
// satisfy a file-system assertion while leaving the account untouched.
func TestEC2Wipe_RemovesInfraAndRetainsBucket(t *testing.T) {
	environment := sharedInstance(t)
	environment.destroy()

	environment.wipeSharedInfrastructure()

	environment.assertSharedInfrastructureIsGone()
	environment.assertStateBucketExists()
}

func (e *ec2Environment) wipeSharedInfrastructure() {
	e.t.Helper()

	output, err := e.runIsolariumEC2Wipe()
	if err != nil {
		e.t.Fatalf("isolarium ec2 wipe: %v\n%s", err, output)
	}
}

func (e *ec2Environment) assertSharedInfrastructureIsGone() {
	e.t.Helper()

	for _, resource := range wipedInfrastructure() {
		e.assertResourceIsGone(resource)
	}
}

// wipedInfrastructure is the subset of the shared resources the observable
// names; the subnet, internet gateway and route table go with the VPC that
// contains them.
func wipedInfrastructure() []sharedResource {
	return []sharedResource{
		{"VPC", countVPCs},
		{"security group", countSecurityGroups},
		{"key pair", countKeyPairs},
	}
}

func (e *ec2Environment) assertResourceIsGone(resource sharedResource) {
	e.t.Helper()

	if count := e.countSharedResource(resource); count != 0 {
		e.t.Errorf("%d isolarium %s resources still exist after wipe, want none", count, resource.label)
	}
}
