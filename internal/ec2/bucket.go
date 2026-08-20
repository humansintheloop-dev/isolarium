package ec2

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type callerIdentityAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type stateBucketAPI interface {
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	PutBucketVersioning(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error)
	PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error)
	PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
}

func StateBucketName(accountID, region string) string {
	return fmt.Sprintf("isolarium-tfstate-%s-%s", accountID, region)
}

func EnsureStateBucket(ctx context.Context, identity callerIdentityAPI, bucketAPI stateBucketAPI, region string) (string, error) {
	accountID, err := resolveAccountID(ctx, identity)
	if err != nil {
		return "", err
	}

	bucket := StateBucketName(accountID, region)
	if err := createBucketIfAbsent(ctx, bucketAPI, bucket, region); err != nil {
		return "", err
	}
	if err := configureStateBucket(ctx, bucketAPI, bucket); err != nil {
		return "", err
	}
	return bucket, nil
}

func resolveAccountID(ctx context.Context, identity callerIdentityAPI) (string, error) {
	out, err := identity.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("resolving AWS account: %w", err)
	}
	if out.Account == nil || *out.Account == "" {
		return "", errors.New("resolving AWS account: GetCallerIdentity returned no account ID")
	}
	return *out.Account, nil
}

func createBucketIfAbsent(ctx context.Context, bucketAPI stateBucketAPI, bucket, region string) error {
	input := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if constraint, needed := locationConstraintFor(region); needed {
		input.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{LocationConstraint: constraint}
	}

	_, err := bucketAPI.CreateBucket(ctx, input)
	if err == nil || bucketIsAlreadyOurs(err) {
		return nil
	}
	return fmt.Errorf("creating state bucket %s: %w", bucket, err)
}

// us-east-1 is the default location and rejects an explicit LocationConstraint.
func locationConstraintFor(region string) (s3types.BucketLocationConstraint, bool) {
	if region == "us-east-1" {
		return "", false
	}
	return s3types.BucketLocationConstraint(region), true
}

func bucketIsAlreadyOurs(err error) bool {
	var owned *s3types.BucketAlreadyOwnedByYou
	return errors.As(err, &owned)
}

func configureStateBucket(ctx context.Context, bucketAPI stateBucketAPI, bucket string) error {
	if err := enableVersioning(ctx, bucketAPI, bucket); err != nil {
		return err
	}
	if err := enableEncryption(ctx, bucketAPI, bucket); err != nil {
		return err
	}
	return blockPublicAccess(ctx, bucketAPI, bucket)
}

func enableVersioning(ctx context.Context, bucketAPI stateBucketAPI, bucket string) error {
	_, err := bucketAPI.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: s3types.BucketVersioningStatusEnabled,
		},
	})
	if err != nil {
		return fmt.Errorf("enabling versioning on %s: %w", bucket, err)
	}
	return nil
}

func enableEncryption(ctx context.Context, bucketAPI stateBucketAPI, bucket string) error {
	_, err := bucketAPI.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucket),
		ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
			Rules: []s3types.ServerSideEncryptionRule{{
				ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
					SSEAlgorithm: s3types.ServerSideEncryptionAes256,
				},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("enabling encryption on %s: %w", bucket, err)
	}
	return nil
}

func blockPublicAccess(ctx context.Context, bucketAPI stateBucketAPI, bucket string) error {
	_, err := bucketAPI.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("blocking public access on %s: %w", bucket, err)
	}
	return nil
}
