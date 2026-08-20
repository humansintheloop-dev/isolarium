//go:build ec2

package ec2_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
	"github.com/humansintheloop-dev/isolarium/internal/git"
	"github.com/humansintheloop-dev/isolarium/internal/github"
)

const (
	lifecycleEnvironmentName = "isolarium-ec2-lifecycle-test"
	integrationGateEnvVar    = "ISOLARIUM_EC2_INTEGRATION"
	sshReadinessTimeout      = 5 * time.Minute
	sshReadinessInterval     = 10 * time.Second
	propagatedExitCode       = 42
)

func TestEC2Lifecycle_CreatesRunsCommandsAndDestroys(t *testing.T) {
	environment := startEC2Environment(t, lifecycleEnvironmentName)

	environment.assertSharedInfrastructureExists()
	environment.assertEchoWritesHelloToStdout()
	environment.assertExitCodeIsPropagated(propagatedExitCode)

	environment.destroy()
	environment.assertInstanceIsTerminated()
}

// ec2Environment is one real instance under test, together with the host state
// its lifecycle wrote and the AWS clients used to observe the account directly.
type ec2Environment struct {
	t          *testing.T
	name       string
	base       string
	region     string
	backend    *backend.EC2Backend
	repository ec2.RepositorySpec
	instanceID string
	publicDNS  string
	createdAt  time.Time
	destroyed  bool
	config     *aws.Config
}

func startEC2Environment(t *testing.T, name string) *ec2Environment {
	t.Helper()

	requireIntegrationGate(t)
	region := requireAWSCredentials(t)

	instance := backend.NewEC2Backend()
	instance.MetadataDir = t.TempDir()

	environment := &ec2Environment{
		t:          t,
		name:       name,
		base:       instance.MetadataDir,
		region:     region,
		backend:    instance,
		repository: integrationRepositorySpec(t),
	}

	t.Cleanup(environment.destroyIfStillRunning)
	environment.create()
	environment.waitForSSH()
	return environment
}

// integrationRepositorySpec resolves the checkout under test exactly as the
// CLI does before create: the repository it belongs to, the branch being worked
// on, and a freshly minted installation token. The branch is pushed so the
// instance has something to clone.
func integrationRepositorySpec(t *testing.T) ec2.RepositorySpec {
	t.Helper()

	checkout := repositoryCheckout(t)
	remoteURL, err := git.GetRemoteURL(checkout)
	if err != nil {
		t.Fatalf("resolving the remote of %s: %v", checkout, err)
	}
	owner, repo, err := github.ParseRepoURL(remoteURL)
	if err != nil {
		t.Fatalf("parsing %s: %v", remoteURL, err)
	}
	branch, err := git.GetCurrentBranch(checkout)
	if err != nil {
		t.Fatalf("resolving the current branch of %s: %v", checkout, err)
	}
	if err := git.PushBranch(checkout, branch); err != nil {
		t.Fatalf("pushing %s so the instance can clone it: %v", branch, err)
	}

	return ec2.RepositorySpec{
		Owner:       owner,
		Repo:        repo,
		Branch:      branch,
		Token:       mintIntegrationToken(t, owner, repo),
		HostDir:     checkout,
		AuthorEmail: hostGitSetting(t, checkout, git.GetUserEmail, "user.email"),
		AuthorName:  hostGitSetting(t, checkout, git.GetUserName, "user.name"),
	}
}

// repositoryCheckout is the root of the working copy, which is where the project
// config files that travel into the instance live — not the package directory
// go test runs from.
func repositoryCheckout(t *testing.T) string {
	t.Helper()

	output, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("resolving the repository checkout: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func mintIntegrationToken(t *testing.T, owner, repo string) string {
	t.Helper()

	appID := requireEnvVar(t, "GITHUB_APP_ID")
	privateKeyPath := requireEnvVar(t, "GITHUB_APP_PRIVATE_KEY_PATH")

	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		t.Fatalf("reading the GitHub App private key from %s: %v", privateKeyPath, err)
	}
	minter, err := github.NewTokenMinter(appID, string(privateKey), "")
	if err != nil {
		t.Fatalf("creating the GitHub App token minter: %v", err)
	}
	token, err := minter.MintInstallationToken(owner, repo)
	if err != nil {
		t.Fatalf("minting an installation token for %s/%s: %v", owner, repo, err)
	}
	return token
}

func hostGitSetting(t *testing.T, checkout string, read func(string) (string, error), key string) string {
	t.Helper()

	value, err := read(checkout)
	if err != nil {
		t.Fatalf("reading git %s: %v", key, err)
	}
	return value
}

func requireIntegrationGate(t *testing.T) {
	t.Helper()

	if os.Getenv(integrationGateEnvVar) != "1" {
		t.Fatalf("%s=1 is required to run EC2 tests", integrationGateEnvVar)
	}
}

func requireAWSCredentials(t *testing.T) string {
	t.Helper()

	region := requireEnvVar(t, "AWS_REGION")
	requireEnvVar(t, "AWS_ACCESS_KEY_ID")
	requireEnvVar(t, "AWS_SECRET_ACCESS_KEY")
	return region
}

func requireEnvVar(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s must be set to run EC2 tests", name)
	}
	return value
}

func (e *ec2Environment) create() {
	e.t.Helper()

	e.createdAt = time.Now()
	if err := e.backend.Create(backend.CreateOptions{Name: e.name, Repository: e.repositorySource()}); err != nil {
		e.t.Fatalf("creating %s: %v", e.name, err)
	}
	e.t.Logf("TIMING: create %s took %s", e.name, time.Since(e.createdAt).Round(time.Second))

	meta, err := ec2.NewMetadataStore(e.base, e.name).Read()
	if err != nil {
		e.t.Fatalf("reading metadata for %s: %v", e.name, err)
	}
	e.instanceID = meta.InstanceID
	e.publicDNS = meta.PublicDNS
}

func (e *ec2Environment) repositorySource() backend.RepositorySource {
	return func() (ec2.RepositorySpec, error) { return e.repository, nil }
}

func (e *ec2Environment) waitForSSH() {
	e.t.Helper()

	deadline := time.Now().Add(sshReadinessTimeout)
	for !e.sshLoginSucceeds() {
		if time.Now().After(deadline) {
			e.t.Fatalf("instance %s did not become reachable over SSH within %s", e.instanceID, sshReadinessTimeout)
		}
		time.Sleep(sshReadinessInterval)
	}
	e.t.Logf("TIMING: cold start from create to first SSH login took %s", time.Since(e.createdAt).Round(time.Second))
}

// sshLoginSucceeds probes reachability through the same command builder Exec
// uses, but discards its output so the retry loop stays quiet.
func (e *ec2Environment) sshLoginSucceeds() bool {
	args := ec2.BuildExecCommand(e.base, e.publicDNS, ec2.RemoteCommand{Args: []string{"true"}})
	return exec.Command(args[0], args[1:]...).Run() == nil
}

func (e *ec2Environment) assertEchoWritesHelloToStdout() {
	e.t.Helper()

	var exitCode int
	var err error
	output := captureStdout(e.t, func() {
		exitCode, err = e.backend.Exec(backend.ExecRequest{
			ContainerName: e.name,
			Args:          []string{"echo", "hello"},
		})
	})

	if err != nil {
		e.t.Fatalf("Exec of echo hello: %v", err)
	}
	if exitCode != 0 {
		e.t.Errorf("Exec of echo hello exit code = %d, want 0", exitCode)
	}
	if strings.TrimSpace(output) != "hello" {
		e.t.Errorf("Exec of echo hello stdout = %q, want %q", output, "hello")
	}
}

func (e *ec2Environment) assertExitCodeIsPropagated(want int) {
	e.t.Helper()

	exitCode, err := e.backend.Exec(backend.ExecRequest{
		ContainerName: e.name,
		Args:          []string{"exit", strconv.Itoa(want)},
	})
	if err != nil {
		e.t.Fatalf("Exec of exit %d: %v", want, err)
	}
	if exitCode != want {
		e.t.Errorf("Exec of exit %d returned %d, want %d", want, exitCode, want)
	}
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

// destroyIfStillRunning tears the instance down even when an assertion has
// already failed, so a red run never leaves an instance billing.
func (e *ec2Environment) destroyIfStillRunning() {
	if e.destroyed {
		return
	}
	if err := e.backend.Destroy(e.name); err != nil {
		e.t.Errorf("cleanup failed to destroy %s; instance %s may still be billing: %v", e.name, e.instanceID, err)
		return
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

func (e *ec2Environment) assertSharedInfrastructureExists() {
	e.t.Helper()

	e.assertStateBucketExists()
	for _, resource := range sharedInfrastructure() {
		e.assertResourceExists(resource)
	}
}

// sharedResource is one class of account-level resource the walking skeleton is
// expected to have provisioned, paired with the way to count it.
type sharedResource struct {
	label string
	count func(ctx context.Context, api *awsec2.Client) (int, error)
}

func sharedInfrastructure() []sharedResource {
	return []sharedResource{
		{"VPC", countVPCs},
		{"subnet", countSubnets},
		{"internet gateway", countInternetGateways},
		{"route table", countRouteTables},
		{"security group", countSecurityGroups},
		{"key pair", countKeyPairs},
	}
}

func (e *ec2Environment) assertResourceExists(resource sharedResource) {
	e.t.Helper()

	count, err := resource.count(context.Background(), e.ec2API())
	if err != nil {
		e.t.Fatalf("describing the isolarium %s: %v", resource.label, err)
	}
	if count == 0 {
		e.t.Errorf("no isolarium %s exists in the account after create", resource.label)
	}
}

func (e *ec2Environment) assertStateBucketExists() {
	e.t.Helper()

	ctx := context.Background()
	identity, err := sts.NewFromConfig(e.awsConfig()).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		e.t.Fatalf("resolving the AWS account: %v", err)
	}

	bucket := ec2.StateBucketName(aws.ToString(identity.Account), e.region)
	_, err = s3.NewFromConfig(e.awsConfig()).HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		e.t.Errorf("state bucket %s does not exist after create: %v", bucket, err)
	}
}

// managedByIsolarium selects only the resources this project's Terraform
// provider tags, so an unrelated VPC in the account can never satisfy an
// assertion.
func managedByIsolarium() []awsec2types.Filter {
	return []awsec2types.Filter{{
		Name:   aws.String("tag:ManagedBy"),
		Values: []string{"isolarium"},
	}}
}

func countVPCs(ctx context.Context, api *awsec2.Client) (int, error) {
	described, err := api.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{Filters: managedByIsolarium()})
	if err != nil {
		return 0, err
	}
	return len(described.Vpcs), nil
}

func countSubnets(ctx context.Context, api *awsec2.Client) (int, error) {
	described, err := api.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{Filters: managedByIsolarium()})
	if err != nil {
		return 0, err
	}
	return len(described.Subnets), nil
}

func countInternetGateways(ctx context.Context, api *awsec2.Client) (int, error) {
	described, err := api.DescribeInternetGateways(ctx, &awsec2.DescribeInternetGatewaysInput{Filters: managedByIsolarium()})
	if err != nil {
		return 0, err
	}
	return len(described.InternetGateways), nil
}

func countRouteTables(ctx context.Context, api *awsec2.Client) (int, error) {
	described, err := api.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{Filters: managedByIsolarium()})
	if err != nil {
		return 0, err
	}
	return len(described.RouteTables), nil
}

func countSecurityGroups(ctx context.Context, api *awsec2.Client) (int, error) {
	described, err := api.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{Filters: managedByIsolarium()})
	if err != nil {
		return 0, err
	}
	return len(described.SecurityGroups), nil
}

func countKeyPairs(ctx context.Context, api *awsec2.Client) (int, error) {
	described, err := api.DescribeKeyPairs(ctx, &awsec2.DescribeKeyPairsInput{Filters: managedByIsolarium()})
	if err != nil {
		return 0, err
	}
	return len(described.KeyPairs), nil
}

func (e *ec2Environment) ec2API() *awsec2.Client {
	return awsec2.NewFromConfig(e.awsConfig())
}

func (e *ec2Environment) awsConfig() aws.Config {
	e.t.Helper()

	if e.config == nil {
		config, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(e.region))
		if err != nil {
			e.t.Fatalf("loading AWS configuration: %v", err)
		}
		e.config = &config
	}
	return *e.config
}

// captureStdout redirects the process-wide stdout that ExecCommand streams to,
// so the remote command's own output can be asserted on.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating a stdout pipe: %v", err)
	}

	original := os.Stdout
	os.Stdout = writer

	captured := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		captured <- string(data)
	}()

	run()

	os.Stdout = original
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the stdout pipe: %v", err)
	}
	output := <-captured
	if err := reader.Close(); err != nil {
		t.Fatalf("closing the stdout pipe reader: %v", err)
	}
	return output
}
