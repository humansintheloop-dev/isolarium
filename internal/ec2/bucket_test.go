package ec2

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type fakeCallerIdentity struct {
	accountID string
	err       error
}

func (f *fakeCallerIdentity) GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &sts.GetCallerIdentityOutput{Account: &f.accountID}, nil
}

type fakeStateBucket struct {
	callLog          []string
	createBucketErr  error
	createInput      *s3.CreateBucketInput
	versioningInput  *s3.PutBucketVersioningInput
	encryptionInput  *s3.PutBucketEncryptionInput
	publicAccessArgs *s3.PutPublicAccessBlockInput
}

func (f *fakeStateBucket) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	f.callLog = append(f.callLog, "CreateBucket")
	f.createInput = params
	if f.createBucketErr != nil {
		return nil, f.createBucketErr
	}
	return &s3.CreateBucketOutput{}, nil
}

func (f *fakeStateBucket) PutBucketVersioning(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
	f.callLog = append(f.callLog, "PutBucketVersioning")
	f.versioningInput = params
	return &s3.PutBucketVersioningOutput{}, nil
}

func (f *fakeStateBucket) PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	f.callLog = append(f.callLog, "PutBucketEncryption")
	f.encryptionInput = params
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (f *fakeStateBucket) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	f.callLog = append(f.callLog, "PutPublicAccessBlock")
	f.publicAccessArgs = params
	return &s3.PutPublicAccessBlockOutput{}, nil
}

const (
	testAccountID  = "123456789012"
	testRegion     = "us-west-2"
	testBucketName = "isolarium-tfstate-123456789012-us-west-2"
)

func assertOrderedCallLog(t *testing.T, got []string) {
	t.Helper()
	want := []string{"CreateBucket", "PutBucketVersioning", "PutBucketEncryption", "PutPublicAccessBlock"}
	if len(got) != len(want) {
		t.Fatalf("call log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call log = %v, want %v", got, want)
		}
	}
}

func assertBucketNameOnEveryCall(t *testing.T, fake *fakeStateBucket) {
	t.Helper()
	inputBuckets := map[string]*string{
		"CreateBucket":         fake.createInput.Bucket,
		"PutBucketVersioning":  fake.versioningInput.Bucket,
		"PutBucketEncryption":  fake.encryptionInput.Bucket,
		"PutPublicAccessBlock": fake.publicAccessArgs.Bucket,
	}
	for call, bucket := range inputBuckets {
		if bucket == nil || *bucket != testBucketName {
			t.Errorf("%s received bucket %v, want %q", call, bucket, testBucketName)
		}
	}
}

func TestStateBucketName(t *testing.T) {
	got := StateBucketName(testAccountID, testRegion)
	if got != testBucketName {
		t.Errorf("StateBucketName() = %q, want %q", got, testBucketName)
	}
}

func TestEnsureStateBucket_CreatesAndConfigures(t *testing.T) {
	identity := &fakeCallerIdentity{accountID: testAccountID}
	bucket := &fakeStateBucket{}

	name, err := EnsureStateBucket(context.Background(), identity, bucket, testRegion)

	if err != nil {
		t.Fatalf("EnsureStateBucket() returned error: %v", err)
	}
	if name != testBucketName {
		t.Errorf("EnsureStateBucket() = %q, want %q", name, testBucketName)
	}
	assertOrderedCallLog(t, bucket.callLog)
	assertBucketNameOnEveryCall(t, bucket)

	if got := bucket.createInput.CreateBucketConfiguration; got == nil || got.LocationConstraint != s3types.BucketLocationConstraint(testRegion) {
		t.Errorf("CreateBucket location constraint = %v, want %q", got, testRegion)
	}
	if got := bucket.versioningInput.VersioningConfiguration.Status; got != s3types.BucketVersioningStatusEnabled {
		t.Errorf("versioning status = %q, want %q", got, s3types.BucketVersioningStatusEnabled)
	}
	rules := bucket.encryptionInput.ServerSideEncryptionConfiguration.Rules
	if len(rules) != 1 || rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm != s3types.ServerSideEncryptionAes256 {
		t.Errorf("encryption rules = %+v, want a single AES256 rule", rules)
	}
	assertAllPublicAccessFlagsTrue(t, bucket.publicAccessArgs.PublicAccessBlockConfiguration)
}

func assertAllPublicAccessFlagsTrue(t *testing.T, config *s3types.PublicAccessBlockConfiguration) {
	t.Helper()
	if config == nil {
		t.Fatal("PutPublicAccessBlock received a nil configuration")
	}
	flags := map[string]*bool{
		"BlockPublicAcls":       config.BlockPublicAcls,
		"BlockPublicPolicy":     config.BlockPublicPolicy,
		"IgnorePublicAcls":      config.IgnorePublicAcls,
		"RestrictPublicBuckets": config.RestrictPublicBuckets,
	}
	for name, flag := range flags {
		if flag == nil || !*flag {
			t.Errorf("%s = %v, want true", name, flag)
		}
	}
}

func TestEnsureStateBucket_IsIdempotent(t *testing.T) {
	identity := &fakeCallerIdentity{accountID: testAccountID}
	bucket := &fakeStateBucket{createBucketErr: &s3types.BucketAlreadyOwnedByYou{}}

	name, err := EnsureStateBucket(context.Background(), identity, bucket, testRegion)

	if err != nil {
		t.Fatalf("EnsureStateBucket() returned error for an already-owned bucket: %v", err)
	}
	if name != testBucketName {
		t.Errorf("EnsureStateBucket() = %q, want %q", name, testBucketName)
	}
	assertOrderedCallLog(t, bucket.callLog)
}

func TestEnsureStateBucket_ReturnsOtherCreateBucketErrors(t *testing.T) {
	identity := &fakeCallerIdentity{accountID: testAccountID}
	accessDenied := errors.New("AccessDenied")
	bucket := &fakeStateBucket{createBucketErr: accessDenied}

	_, err := EnsureStateBucket(context.Background(), identity, bucket, testRegion)

	if !errors.Is(err, accessDenied) {
		t.Fatalf("EnsureStateBucket() error = %v, want it to wrap %v", err, accessDenied)
	}
	if len(bucket.callLog) != 1 {
		t.Errorf("call log = %v, want only CreateBucket", bucket.callLog)
	}
}

func TestEnsureStateBucket_OmitsLocationConstraintForUSEast1(t *testing.T) {
	identity := &fakeCallerIdentity{accountID: testAccountID}
	bucket := &fakeStateBucket{}

	name, err := EnsureStateBucket(context.Background(), identity, bucket, "us-east-1")

	if err != nil {
		t.Fatalf("EnsureStateBucket() returned error: %v", err)
	}
	if name != "isolarium-tfstate-123456789012-us-east-1" {
		t.Errorf("EnsureStateBucket() = %q, want %q", name, "isolarium-tfstate-123456789012-us-east-1")
	}
	if bucket.createInput.CreateBucketConfiguration != nil {
		t.Errorf("CreateBucket configuration = %+v, want nil for us-east-1", bucket.createInput.CreateBucketConfiguration)
	}
}

func TestEnsureStateBucket_ReturnsCallerIdentityError(t *testing.T) {
	expiredToken := errors.New("ExpiredToken")
	identity := &fakeCallerIdentity{err: expiredToken}
	bucket := &fakeStateBucket{}

	_, err := EnsureStateBucket(context.Background(), identity, bucket, testRegion)

	if !errors.Is(err, expiredToken) {
		t.Fatalf("EnsureStateBucket() error = %v, want it to wrap %v", err, expiredToken)
	}
	if len(bucket.callLog) != 0 {
		t.Errorf("call log = %v, want no S3 calls", bucket.callLog)
	}
}
