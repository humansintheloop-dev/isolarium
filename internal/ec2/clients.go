package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func loadRegionalConfig(ctx context.Context, region string) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS configuration: %w", err)
	}
	return cfg, nil
}

func newInstanceClient(ctx context.Context, region string) (describeInstancesAPI, error) {
	cfg, err := loadRegionalConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	return awsec2.NewFromConfig(cfg), nil
}

func newStateBucketClients(ctx context.Context, region string) (callerIdentityAPI, stateBucketAPI, error) {
	cfg, err := loadRegionalConfig(ctx, region)
	if err != nil {
		return nil, nil, err
	}
	return sts.NewFromConfig(cfg), s3.NewFromConfig(cfg), nil
}

// BootstrapStateBucket builds SDK clients from the ambient AWS environment and
// ensures the Terraform remote-state bucket exists and is configured.
func BootstrapStateBucket(ctx context.Context, region string) (string, error) {
	identity, bucketAPI, err := newStateBucketClients(ctx, region)
	if err != nil {
		return "", err
	}
	return EnsureStateBucket(ctx, identity, bucketAPI, region)
}
