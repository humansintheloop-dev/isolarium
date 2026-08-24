package ec2

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeDescribeInstances struct {
	input     *awsec2.DescribeInstancesInput
	publicDNS string
	state     string
	empty     bool
	err       error
}

func (f *fakeDescribeInstances) DescribeInstances(ctx context.Context, params *awsec2.DescribeInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
	f.input = params
	if f.err != nil {
		return nil, f.err
	}
	if f.empty {
		return &awsec2.DescribeInstancesOutput{}, nil
	}
	return &awsec2.DescribeInstancesOutput{
		Reservations: []awsec2types.Reservation{{
			Instances: []awsec2types.Instance{{
				PublicDnsName: aws.String(f.publicDNS),
				State:         &awsec2types.InstanceState{Name: awsec2types.InstanceStateName(f.state)},
			}},
		}},
	}, nil
}

func TestDescribeInstance_ReturnsPublicDNSAndState(t *testing.T) {
	api := &fakeDescribeInstances{
		publicDNS: "ec2-203-0-113-7.compute-1.amazonaws.com",
		state:     "running",
	}

	publicDNS, state, err := DescribeInstance(context.Background(), api, "i-0123456789abcdef0")

	if err != nil {
		t.Fatalf("DescribeInstance() error = %v, want nil", err)
	}
	if publicDNS != "ec2-203-0-113-7.compute-1.amazonaws.com" {
		t.Errorf("DescribeInstance() public DNS = %q, want %q", publicDNS, "ec2-203-0-113-7.compute-1.amazonaws.com")
	}
	if state != "running" {
		t.Errorf("DescribeInstance() state = %q, want %q", state, "running")
	}
	assertDescribedInstanceID(t, api, "i-0123456789abcdef0")
}

func assertDescribedInstanceID(t *testing.T, api *fakeDescribeInstances, want string) {
	t.Helper()

	if api.input == nil {
		t.Fatal("DescribeInstance() made no DescribeInstances call")
	}
	if len(api.input.InstanceIds) != 1 || api.input.InstanceIds[0] != want {
		t.Errorf("DescribeInstance() asked about %v, want [%s]", api.input.InstanceIds, want)
	}
}

func TestDescribeInstance_ReturnsTheAWSError(t *testing.T) {
	noCredentials := errors.New("NoCredentialProviders: no valid providers in chain")
	api := &fakeDescribeInstances{err: noCredentials}

	_, _, err := DescribeInstance(context.Background(), api, "i-0123456789abcdef0")

	if !errors.Is(err, noCredentials) {
		t.Fatalf("DescribeInstance() error = %v, want it to wrap %v", err, noCredentials)
	}
}

func TestDescribeInstance_FailsWhenAWSKnowsNoSuchInstance(t *testing.T) {
	api := &fakeDescribeInstances{empty: true}

	_, _, err := DescribeInstance(context.Background(), api, "i-0123456789abcdef0")

	if err == nil {
		t.Fatal("DescribeInstance() returned nil error for a reservation-less response")
	}
}
