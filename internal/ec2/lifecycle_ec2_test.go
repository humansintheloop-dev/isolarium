//go:build ec2

package ec2_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	sshReadinessTimeout   = 5 * time.Minute
	sshReadinessInterval  = 10 * time.Second
	propagatedExitCode    = 42

	// createCommandExitCode is what the command handed to `run --create` exits
	// with. It is deliberately non-zero and deliberately not 1, so that the
	// status coming back can only be the command's own, carried through the
	// tmux session it ran in, and never the binary's failure exit.
	createCommandExitCode = 3
)

// The whole ec2 suite works against a single billable instance, so the order the
// tests run in is part of the design rather than an accident:
//
//   - go test runs the tests of one package in source order within a file, and
//     walks the files in sorted-filename order. This file sorts first among the
//     files holding tests, so TestEC2Lifecycle_Creates is what actually creates
//     the instance; the terminate test lives in the file that sorts last.
//   - repo_ec2_test.go sorts ahead of run_ec2_test.go, session_ec2_test.go and
//     tmux_ec2_test.go on purpose: its clone-cleanliness and token-grep
//     assertions need an instance nothing has written to yet, and those later
//     files copy the Claude credentials in and place a script in the home
//     directory.
//   - recovery_ec2_test.go sorts between this file and repo_ec2_test.go, which
//     is where its stop and start belong: the instance already exists, and no
//     test has yet built up the tmux state a reboot would throw away.
//   - run_ec2_test.go and run_hostaddress_ec2_test.go sort ahead of
//     tmux_ec2_test.go because they need the shared tmux session free, and the
//     tmux tests deliberately leave a process running in it.
//
// Renaming one of those files, or adding a test that writes to the instance
// ahead of them, breaks assertions elsewhere without breaking this test.
//
// The instance is created the way i2code creates one: through the built
// binary's `run --create`, which launches the environment because none exists
// and then runs the command inside it. The create helper asserts that the
// command's own exit status came back, so a full run proves the i2code entry
// point against a fresh name before anything else is asked of the instance.
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
	hostGitConfigPath, err = resolveHostGitConfigPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	status := m.Run()
	if err := terminateSharedInstance(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	fmt.Fprintf(os.Stderr, "the suite's working directory was left at %s\n", dir)
	if err := unstageProjectConfig(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	if hostMarkerDir != "" {
		if err := os.RemoveAll(hostMarkerDir); err != nil {
			fmt.Fprintf(os.Stderr, "removing %s: %v\n", hostMarkerDir, err)
			status = 1
		}
	}
	return status
}

// devHomeDirName is the fixed directory the suite stands in for the host's home
// directory with. A per-run temp directory put the working directory at an
// unpredictable path under /var/folders and abandoned one there for every run
// that was killed -- which is exactly when the working directory is worth
// reading. Note that a run reaching the end still wipes most of what is here,
// because the last test runs isolarium ec2 wipe and that removes the Terraform
// directory and the keypair by design; what the fixed path buys is somewhere
// known to look, and the remains of a run that did not get that far.
// It is gitignored.
const devHomeDirName = "terraform-dev"

func makeSharedHomeDir() (string, error) {
	checkout, err := resolveRepositoryCheckout()
	if err != nil {
		return "", err
	}

	sharedHomeDir = filepath.Join(checkout, devHomeDirName)
	sharedMetadataDir = filepath.Join(sharedHomeDir, ".isolarium")
	if err := os.MkdirAll(sharedMetadataDir, 0o755); err != nil {
		return "", err
	}
	return sharedHomeDir, discardEnvironmentsAnEarlierRunLeft()
}

// discardEnvironmentsAnEarlierRunLeft removes the instance declarations and
// metadata that a killed run leaves in a directory which now survives it. They
// would otherwise make create refuse to overwrite an existing instance, and wipe
// refuse to run at all, on every later run. What makes the directory worth
// keeping -- the provider downloads, the backend initialisation and the keypair
// -- is deliberately left alone.
func discardEnvironmentsAnEarlierRunLeft() error {
	names, err := ec2.ListInstanceNames(sharedMetadataDir)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := ec2.RemoveInstanceFile(sharedMetadataDir, name); err != nil {
			return err
		}
		if err := ec2.NewMetadataStore(sharedMetadataDir, name).Cleanup(); err != nil {
			return err
		}
	}
	return nil
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
	repository hostRepository
	instanceID string
	publicDNS  string
	createdAt  time.Time
	destroyed  bool
	config     *aws.Config
}

// newEC2Environment resolves everything create needs without launching anything,
// so a missing gate, a missing credential or an unresolvable checkout fails
// before the account is touched. The branch is pushed, and the clone token
// minted, by the binary itself when it creates — which is what a user's create
// does too.
func newEC2Environment(t *testing.T, name string) *ec2Environment {
	t.Helper()

	region := requireAWSCredentials(t)
	requireGitHubApp(t)

	instance := backend.NewEC2Backend()
	instance.MetadataDir = sharedMetadataDir

	checkout := repositoryCheckout(t)
	workDir := pidScriptWorkDirectory(t)
	stageProjectConfigInTheWorkDirectory(t, checkout, workDir)

	return &ec2Environment{
		t:          t,
		name:       name,
		base:       instance.MetadataDir,
		workDir:    workDir,
		region:     region,
		backend:    instance,
		repository: resolveHostRepository(t, checkout),
	}
}

// hostRepository is what the suite knows about the checkout the binary creates
// from, resolved the same way the binary resolves it so the assertions about
// the instance's clone have something to compare against.
type hostRepository struct {
	owner       string
	repo        string
	branch      string
	authorEmail string
	authorName  string
}

func resolveHostRepository(t *testing.T, checkout string) hostRepository {
	t.Helper()

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

	return hostRepository{
		owner:       owner,
		repo:        repo,
		branch:      branch,
		authorEmail: hostGitSetting(t, checkout, git.GetUserEmail, "user.email"),
		authorName:  hostGitSetting(t, checkout, git.GetUserName, "user.name"),
	}
}

// repositoryCheckout is the root of the working copy, which is where the project
// config files that travel into the instance live — not the package directory
// go test runs from.
func repositoryCheckout(t *testing.T) string {
	t.Helper()

	checkout, err := resolveRepositoryCheckout()
	if err != nil {
		t.Fatalf("resolving the repository checkout: %v", err)
	}
	return checkout
}

// resolveRepositoryCheckout is what repositoryCheckout reports through a
// *testing.T, separated out because TestMain has no test to fail.
func resolveRepositoryCheckout() (string, error) {
	output, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("resolving the repository checkout: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// stagedProjectConfig is what the suite copied from the checkout into the work
// directory, so it can be removed again once the run is over.
var stagedProjectConfig []string

// stageProjectConfigInTheWorkDirectory gives the work directory the project
// config the checkout has, because create copies `.claude/settings.local.json`
// and `CLAUDE.md` from the directory the binary runs in — the pid.yaml fixture
// here, which carries neither of its own. In ordinary use that directory is the
// repository root and the two are the same place. The copies are byte for byte
// the checkout's, so the instance's clone is left unmodified by them, and they
// are gitignored so a killed run cannot leave them looking like a change.
func stageProjectConfigInTheWorkDirectory(t *testing.T, checkout, workDir string) {
	t.Helper()

	for _, name := range projectConfigFiles() {
		contents, err := os.ReadFile(filepath.Join(checkout, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("reading %s from the checkout: %v", name, err)
		}
		staged := filepath.Join(workDir, name)
		if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
			t.Fatalf("staging %s into the work directory: %v", name, err)
		}
		if err := os.WriteFile(staged, contents, 0o644); err != nil {
			t.Fatalf("staging %s into the work directory: %v", name, err)
		}
		stagedProjectConfig = append(stagedProjectConfig, staged)
	}
}

func unstageProjectConfig() error {
	for _, staged := range stagedProjectConfig {
		if err := os.Remove(staged); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing the staged %s: %w", staged, err)
		}
		// The directory a staged file was written into is removed only when it is
		// left empty, so a fixture directory that carries other files survives.
		_ = os.Remove(filepath.Dir(staged))
	}
	return nil
}

func hostGitSetting(t *testing.T, checkout string, read func(string) (string, error), key string) string {
	t.Helper()

	value, err := read(checkout)
	if err != nil {
		t.Fatalf("reading git %s: %v", key, err)
	}
	return value
}

func requireAWSCredentials(t *testing.T) string {
	t.Helper()

	region := requireEnvVar(t, "AWS_REGION")
	requireEnvVar(t, "AWS_ACCESS_KEY_ID")
	requireEnvVar(t, "AWS_SECRET_ACCESS_KEY")
	return region
}

// requireGitHubApp fails the run up front when the binary would be unable to
// mint the clone token, rather than leaving that to surface minutes into a
// create that has already launched an instance.
func requireGitHubApp(t *testing.T) {
	t.Helper()

	requireEnvVar(t, "GITHUB_APP_ID")
	requireEnvVar(t, "GITHUB_APP_PRIVATE_KEY_PATH")
}

func requireEnvVar(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s must be set to run EC2 tests", name)
	}
	return value
}

// create launches the shared instance the way i2code launches one: through the
// built binary's `run --create`, on a name nothing exists under yet. The command
// it is given exits with createCommandExitCode, so the status coming back is the
// proof of both halves at once — the environment was created, and the command's
// own non-zero status travelled back through the tmux session it ran in.
func (e *ec2Environment) create() {
	e.t.Helper()

	e.createdAt = time.Now()
	exitCode, output := e.runIsolarium(e.runArgs(true, "sh", "-c", "exit "+strconv.Itoa(createCommandExitCode))...)
	if exitCode != createCommandExitCode {
		e.t.Fatalf("isolarium run --create --type ec2 --name %s -- sh -c 'exit %d' exited %d, want %d — the command's own status\n%s",
			e.name, createCommandExitCode, exitCode, createCommandExitCode, output)
	}
	e.t.Logf("TIMING: create %s took %s", e.name, time.Since(e.createdAt).Round(time.Second))

	meta, err := ec2.NewMetadataStore(e.base, e.name).Read()
	if err != nil {
		e.t.Fatalf("reading metadata for %s: %v", e.name, err)
	}
	e.instanceID = meta.InstanceID
	e.publicDNS = meta.PublicDNS
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

// sshLoginSucceedsAt probes reachability through the same command builder the
// capture transport uses, but discards its output so the retry loop stays quiet.
func (e *ec2Environment) sshLoginSucceedsAt(publicDNS string) bool {
	args := ec2.BuildExecCommand(e.base, publicDNS, ec2.RemoteCommand{Args: []string{"true"}})
	return exec.Command(args[0], args[1:]...).Run() == nil
}

// assertEchoWritesHelloToStdout drives Exec, which runs the command inside the
// instance's tmux session and streams what the session draws. The text arrives
// among the terminal control sequences tmux uses to paint it, so the assertion
// is that hello is visible in the output rather than that it is the whole of it.
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
	if !strings.Contains(visibleText(output), "hello") {
		e.t.Errorf("Exec of echo hello streamed %q, want it to show %q", visibleText(output), "hello")
	}
}

// assertExitCodeIsPropagated asks a shell to exit with the status, rather than
// running a bare `exit`, because Exec runs the command under a shell of its own
// that records the status once the command returns — a bare exit would end that
// shell before it could.
func (e *ec2Environment) assertExitCodeIsPropagated(want int) {
	e.t.Helper()

	exitCode, err := e.backend.Exec(backend.ExecRequest{
		ContainerName: e.name,
		Args:          []string{"sh", "-c", "exit " + strconv.Itoa(want)},
	})
	if err != nil {
		e.t.Fatalf("Exec of sh -c 'exit %d': %v", want, err)
	}
	if exitCode != want {
		e.t.Errorf("Exec of sh -c 'exit %d' returned %d, want %d", want, exitCode, want)
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

	bucket := e.stateBucketName()
	_, err := s3.NewFromConfig(e.awsConfig()).HeadBucket(context.Background(), &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		e.t.Errorf("HeadBucket on the state bucket %s failed: %v", bucket, err)
	}
}

// stateBucketName is the remote-state bucket create provisioned, named the way
// the product names it from the account and region.
func (e *ec2Environment) stateBucketName() string {
	e.t.Helper()

	identity, err := sts.NewFromConfig(e.awsConfig()).GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		e.t.Fatalf("resolving the AWS account: %v", err)
	}
	return ec2.StateBucketName(aws.ToString(identity.Account), e.region)
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

// instanceOutput runs args on the instance and hands back what it wrote, failing
// the test when the command did not exit 0.
func (e *ec2Environment) instanceOutput(args ...string) string {
	e.t.Helper()

	exitCode, output := e.askInstance(args...)
	if exitCode != 0 {
		e.t.Fatalf("%s on the instance exited %d, want 0; output: %s", strings.Join(args, " "), exitCode, output)
	}
	return strings.TrimSpace(output)
}

// askInstance runs args on the instance over the plain SSH transport and hands
// back its exit status together with what it wrote. It is how the suite
// observes the instance: the same non-interactive shell the product's own
// probes and scripts run in, with none of the tmux rendering that Exec streams,
// so what comes back can be compared exactly. It collects the output into a
// buffer of its own rather than by borrowing the process-wide stdout, because
// the tmux tests read the instance while interactive connections are being
// opened — and a connection that started mid-read would inherit the borrowed
// stdout in place of its terminal and hold it open for as long as the session
// lived.
func (e *ec2Environment) askInstance(args ...string) (int, string) {
	e.t.Helper()

	return e.askInstanceIn("", args...)
}

// askInstanceInRepo is askInstance run inside the clone, which is where Exec
// runs a user's command and so where the assertions about the checkout belong.
// The plain transport lands in the home directory, not there.
func (e *ec2Environment) askInstanceInRepo(args ...string) (int, string) {
	e.t.Helper()

	return e.askInstanceIn(ec2.RemoteRepoDir, args...)
}

func (e *ec2Environment) askInstanceIn(workdir string, args ...string) (int, string) {
	e.t.Helper()

	output, exitCode, err := ec2.CaptureCommand(e.base, e.publicDNS, ec2.RemoteCommand{Workdir: workdir, Args: args})
	if err != nil {
		e.t.Fatalf("running %s on the instance: %v", strings.Join(args, " "), err)
	}
	return exitCode, output
}

// captureStdout redirects the process-wide stdout that Exec streams to, so the
// remote command's own output can be asserted on.
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
