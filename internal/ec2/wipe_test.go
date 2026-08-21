package ec2

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

const (
	wipeTestPublicKey   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfake isolarium"
	wipeTestIngressCIDR = "203.0.113.7/32"
)

type wipeFixture struct {
	base   string
	runner *command.FakeRunner
	out    *bytes.Buffer
}

func newWipeFixture(t *testing.T) wipeFixture {
	t.Helper()

	base := t.TempDir()
	runner := command.NewFakeRunner(t)
	runner.OnCommand("terraform").Returns("")
	return wipeFixture{base: base, runner: runner, out: &bytes.Buffer{}}
}

func (f wipeFixture) deps() WipeDeps {
	return WipeDeps{
		Runner: f.runner,
		ResolveAccountFunc: func() (string, string, error) {
			return testRegion, testBucketName, nil
		},
		EnsureKeypairFunc:  func(string) (string, error) { return wipeTestPublicKey, nil },
		DetectPublicIPFunc: func() (string, error) { return wipeTestIngressCIDR, nil },
		Out:                f.out,
	}
}

func (f wipeFixture) writeInstanceFiles(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		if err := WriteInstanceFile(f.base, name, "#cloud-config\n"); err != nil {
			t.Fatalf("writing instance file for %q: %v", name, err)
		}
	}
}

func (f wipeFixture) writeSharedInfrastructureFiles(t *testing.T) {
	t.Helper()

	if err := os.MkdirAll(TerraformDir(f.base), scaffoldingDirMode); err != nil {
		t.Fatalf("creating %s: %v", TerraformDir(f.base), err)
	}
	markAlreadyInitialised(t, f.base)
	for _, path := range []string{PrivateKeyPath(f.base), PublicKeyPath(f.base), KnownHostsPath(f.base)} {
		if err := os.WriteFile(path, []byte("placeholder\n"), scaffoldingFileMode); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
}

func TestEC2Wipe_RefusesWhenInstancesExist(t *testing.T) {
	fixture := newWipeFixture(t)
	fixture.writeSharedInfrastructureFiles(t)
	fixture.writeInstanceFiles(t, "my-work", "other")

	err := Wipe(fixture.base, fixture.deps())

	if err == nil {
		t.Fatal("Wipe() returned nil error while instance files exist")
	}
	assertContainsAll(t, "the refusal", err.Error(),
		"my-work",
		"other",
		"isolarium destroy --type ec2 --name my-work",
		"isolarium destroy --type ec2 --name other",
	)
	if calls := fixture.runner.Calls(); len(calls) != 0 {
		t.Errorf("Wipe() ran %v, want no terraform invocation", calls)
	}
	assertPathExists(t, TerraformDir(fixture.base))
	assertPathExists(t, PrivateKeyPath(fixture.base))
}

func TestEC2Wipe_TearsDownAndReportsRetainedBucket(t *testing.T) {
	fixture := newWipeFixture(t)
	fixture.writeSharedInfrastructureFiles(t)

	if err := Wipe(fixture.base, fixture.deps()); err != nil {
		t.Fatalf("Wipe() error = %v", err)
	}

	assertSingleCall(t, fixture.runner, strings.Join([]string{
		"terraform",
		"-chdir=" + TerraformDir(fixture.base),
		"destroy",
		"-auto-approve",
		"-input=false",
		"-lock-timeout=120s",
		"-var=ingress_cidr=" + wipeTestIngressCIDR,
		"-var=public_key=" + wipeTestPublicKey,
		"-var=region=" + testRegion,
	}, " "))
	assertPathAbsent(t, TerraformDir(fixture.base))
	assertPathAbsent(t, PrivateKeyPath(fixture.base))
	assertPathAbsent(t, PublicKeyPath(fixture.base))
	assertPathAbsent(t, KnownHostsPath(fixture.base))
	assertContainsAll(t, "the wipe report", fixture.out.String(),
		"S3 state bucket "+testBucketName+" was intentionally retained; see the README for manual removal",
	)
}

func TestEC2Wipe_FallsBackToThePersistedIngressCIDRWhenDetectionFails(t *testing.T) {
	fixture := newWipeFixture(t)
	fixture.writeSharedInfrastructureFiles(t)
	if err := PersistIngressCIDR(fixture.base, "198.51.100.4/32"); err != nil {
		t.Fatalf("persisting the ingress CIDR: %v", err)
	}

	deps := fixture.deps()
	deps.DetectPublicIPFunc = func() (string, error) { return "", os.ErrDeadlineExceeded }

	if err := Wipe(fixture.base, deps); err != nil {
		t.Fatalf("Wipe() error = %v", err)
	}

	if got := strings.Join(fixture.runner.Calls()[0], " "); !strings.Contains(got, "-var=ingress_cidr=198.51.100.4/32") {
		t.Errorf("invocation = %q, want it to carry the persisted ingress CIDR", got)
	}
	assertContainsAll(t, "the wipe report", fixture.out.String(), "warning: public IP detection failed")
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("stat %s: %v, want it to still exist", path, err)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists, want it removed", path)
	}
}
