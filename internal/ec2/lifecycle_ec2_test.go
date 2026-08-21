//go:build ec2

package ec2_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	sharedEnvironmentName = "isolarium-ec2-test"
	integrationGateEnvVar = "ISOLARIUM_EC2_INTEGRATION"
	sshReadinessTimeout   = 5 * time.Minute
	sshReadinessInterval  = 10 * time.Second
	propagatedExitCode    = 42
)

// The whole ec2 suite works against a single billable instance, so the order the
// tests run in is part of the design rather than an accident:
//
//   - go test runs the tests of one package in source order within a file, and
//     walks the files in sorted-filename order. This file sorts first, so
//     TestEC2Lifecycle_Creates is what actually creates the instance; the
//     terminate test lives in the file that sorts last.
//   - repo_ec2_test.go sorts ahead of session_ec2_test.go and tmux_ec2_test.go
//     on purpose: its clone-cleanliness and token-grep assertions need an
//     instance nothing has written to yet, and those two later files copy the
//     Claude credentials in and place a script in the home directory.
//   - recovery_ec2_test.go sorts between this file and repo_ec2_test.go, which
//     is where its stop and start belong: the instance already exists, and no
//     test has yet built up the tmux state a reboot would throw away.
//
// Renaming one of those files, or adding a test that writes to the instance
// ahead of them, breaks assertions elsewhere without breaking this test.
func TestEC2Lifecycle_Creates(t *testing.T) {
	environment := sharedInstance(t)

	environment.assertSharedInfrastructureExists()
	environment.assertEchoWritesHelloToStdout()
	environment.assertExitCodeIsPropagated(propagatedExitCode)
}

// sharedHomeDir stands in for the host's home directory whenever the suite
// drives the built binary, which resolves its own metadata directory from HOME
// rather than being told where to look.
var sharedHomeDir string

// sharedMetadataDir holds the keypair, the known_hosts entry, the Terraform
// state and the instance metadata of the one instance the suite runs against.
// All of it has to outlive the test that created the instance, so it cannot be
// a t.TempDir(). It sits at the path the CLI derives from sharedHomeDir, so the
// binary and the backend the tests construct directly agree on where it is.
var sharedMetadataDir string

// sharedEnvironment is the instance every ec2 test works against, created by
// whichever test asks for it first.
var sharedEnvironment *ec2Environment

func TestMain(m *testing.M) {
	os.Exit(runSuiteAgainstOneInstance(m))
}

func runSuiteAgainstOneInstance(m *testing.M) int {
	dir, err := makeSharedHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating the shared metadata directory: %v\n", err)
		return 1
	}

	status := m.Run()
	if err := terminateSharedInstance(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	for _, temporary := range []string{dir, hostMarkerDir} {
		if temporary == "" {
			continue
		}
		if err := os.RemoveAll(temporary); err != nil {
			fmt.Fprintf(os.Stderr, "removing %s: %v\n", temporary, err)
			status = 1
		}
	}
	return status
}

func makeSharedHomeDir() (string, error) {
	home, err := os.MkdirTemp("", "isolarium-ec2-suite")
	if err != nil {
		return "", err
	}
	sharedHomeDir = home
	sharedMetadataDir = filepath.Join(home, ".isolarium")
	if err := os.MkdirAll(sharedMetadataDir, 0o755); err != nil {
		return "", err
	}
	return home, nil
}

// terminateSharedInstance runs once every test has returned, so a run narrowed
// to a single test — or one abandoned after a failure — never leaves an instance
// billing. A run that reached TestEC2Lifecycle_Terminates has nothing left to do
// here.
func terminateSharedInstance() error {
	if sharedEnvironment == nil || sharedEnvironment.destroyed {
		return nil
	}
	if err := sharedEnvironment.backend.Destroy(sharedEnvironment.name); err != nil {
		return fmt.Errorf("teardown failed to destroy %s; instance %s may still be billing: %w",
			sharedEnvironment.name, sharedEnvironment.instanceID, err)
	}
	sharedEnvironment.destroyed = true
	return nil
}

// sharedInstance hands the running test the instance the suite works against,
// creating it on first use so that any one test still runs alone under
// `./test-scripts/test-ec2.sh <pattern>`. The *testing.T is rebound on every
// access, because the instance outlives the test that created it and its helpers
// must never report against a test that has already returned.
func sharedInstance(t *testing.T) *ec2Environment {
	t.Helper()

	if sharedEnvironment == nil {
		sharedEnvironment = newEC2Environment(t, sharedEnvironmentName)
		sharedEnvironment.create()
		sharedEnvironment.waitForSSH()
	}
	sharedEnvironment.t = t
	return sharedEnvironment
}

// ec2Environment is one real instance under test, together with the host state
// its lifecycle wrote and the AWS clients used to observe the account directly.
type ec2Environment struct {
	t          *testing.T
	name       string
	base       string
	workDir    string
	region     string
	backend    *backend.EC2Backend
	repository ec2.RepositorySpec
	instanceID string
	publicDNS  string
	createdAt  time.Time
	destroyed  bool
	config     *aws.Config
}

// newEC2Environment resolves everything create needs without launching anything,
// so a missing gate, a missing credential or an unpushable branch fails before
// the account is touched.
func newEC2Environment(t *testing.T, name string) *ec2Environment {
	t.Helper()

	requireIntegrationGate(t)
	region := requireAWSCredentials(t)

	instance := backend.NewEC2Backend()
	instance.MetadataDir = sharedMetadataDir

	return &ec2Environment{
		t:          t,
		name:       name,
		base:       instance.MetadataDir,
		workDir:    pidScriptWorkDirectory(t),
		region:     region,
		backend:    instance,
		repository: integrationRepositorySpec(t),
	}
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
	createOptions := backend.CreateOptions{
		Name:          e.name,
		WorkDirectory: e.workDir,
		Repository:    e.repositorySource(),
	}
	if err := e.backend.Create(createOptions); err != nil {
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

	e.waitForSSHAt(e.publicDNS)
	e.t.Logf("TIMING: cold start from create to first SSH login took %s", time.Since(e.createdAt).Round(time.Second))
}

// waitForSSHAt takes an address rather than reading the recorded one, because a
// restarted instance is reachable at a name isolarium has not been told about
// yet.
func (e *ec2Environment) waitForSSHAt(publicDNS string) {
	e.t.Helper()

	deadline := time.Now().Add(sshReadinessTimeout)
	for !e.sshLoginSucceedsAt(publicDNS) {
		if time.Now().After(deadline) {
			e.t.Fatalf("instance %s did not become reachable over SSH at %s within %s", e.instanceID, publicDNS, sshReadinessTimeout)
		}
		time.Sleep(sshReadinessInterval)
	}
}

// sshLoginSucceedsAt probes reachability through the same command builder Exec
// uses, but discards its output so the retry loop stays quiet.
func (e *ec2Environment) sshLoginSucceedsAt(publicDNS string) bool {
	args := ec2.BuildExecCommand(e.base, publicDNS, ec2.RemoteCommand{Args: []string{"true"}})
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

	if e.countSharedResource(resource) == 0 {
		e.t.Errorf("no isolarium %s exists in the account after create", resource.label)
	}
}

func (e *ec2Environment) countSharedResource(resource sharedResource) int {
	e.t.Helper()

	count, err := resource.count(context.Background(), e.ec2API())
	if err != nil {
		e.t.Fatalf("describing the isolarium %s: %v", resource.label, err)
	}
	return count
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
		e.t.Errorf("HeadBucket on the state bucket %s failed: %v", bucket, err)
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
