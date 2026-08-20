package ec2

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func newStateBucketClients(ctx context.Context, region string) (callerIdentityAPI, stateBucketAPI, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, nil, fmt.Errorf("loading AWS configuration: %w", err)
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
