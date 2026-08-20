package ec2

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type describeInstancesAPI interface {
	DescribeInstances(ctx context.Context, params *awsec2.DescribeInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error)
}

// DescribeInstance answers where an instance can currently be reached and what
// state AWS holds it in. The instance ID is the durable handle: the public DNS
// changes across a stop and start, which is what makes the lookup worth making.
func DescribeInstance(ctx context.Context, api describeInstancesAPI, instanceID string) (publicDNS, state string, err error) {
	out, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil {
		return "", "", fmt.Errorf("describing instance %s: %w", instanceID, err)
	}

	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			return dnsName(instance.PublicDnsName), instanceStateName(instance), nil
		}
	}
	return "", "", fmt.Errorf("describing instance %s: AWS returned no such instance", instanceID)
}

func dnsName(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func instanceStateName(instance awsec2types.Instance) string {
	if instance.State == nil {
		return ""
	}
	return string(instance.State.Name)
}

// LookupInstance builds an EC2 client from the ambient AWS environment and
// describes one instance in the region its metadata recorded.
func LookupInstance(ctx context.Context, region, instanceID string) (publicDNS, state string, err error) {
	api, err := newInstanceClient(ctx, region)
	if err != nil {
		return "", "", err
	}
	return DescribeInstance(ctx, api, instanceID)
}
