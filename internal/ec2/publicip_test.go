package ec2

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func fakeCheckIPReturning(body string) HTTPGetFunc {
	return func(url string) (string, error) {
		if url != checkIPURL {
			return "", errors.New("unexpected URL " + url)
		}
		return body, nil
	}
}

func failingCheckIP(message string) HTTPGetFunc {
	return func(string) (string, error) {
		return "", errors.New(message)
	}
}

func TestIngressDetectsAndPersistsCIDR(t *testing.T) {
	base := t.TempDir()

	cidr, err := DetectPublicIP(fakeCheckIPReturning("203.0.113.7\n"))
	if err != nil {
		t.Fatalf("DetectPublicIP() error = %v", err)
	}
	if cidr != "203.0.113.7/32" {
		t.Fatalf("DetectPublicIP() = %q, want %q", cidr, "203.0.113.7/32")
	}

	if err := PersistIngressCIDR(base, cidr); err != nil {
		t.Fatalf("PersistIngressCIDR() error = %v", err)
	}

	contents := readFile(t, TfvarsPath(base))
	if !strings.Contains(contents, `ingress_cidr = "203.0.113.7/32"`) {
		t.Errorf("isolarium.auto.tfvars = %q, want it to contain %q", contents, `ingress_cidr = "203.0.113.7/32"`)
	}

	persisted, err := ReadPersistedIngressCIDR(base)
	if err != nil {
		t.Fatalf("ReadPersistedIngressCIDR() error = %v", err)
	}
	if persisted != "203.0.113.7/32" {
		t.Errorf("ReadPersistedIngressCIDR() = %q, want %q", persisted, "203.0.113.7/32")
	}
}

func TestIngressNeverReturnsOpenCIDR(t *testing.T) {
	unusableResponses := map[string]HTTPGetFunc{
		"transport failure": failingCheckIP("dial tcp: no route to host"),
		"empty body":        fakeCheckIPReturning("\n"),
		"not an address":    fakeCheckIPReturning("<html>captive portal</html>\n"),
		"already a CIDR":    fakeCheckIPReturning("0.0.0.0/0\n"),
		"unspecified IPv4":  fakeCheckIPReturning("0.0.0.0\n"),
		"loopback":          fakeCheckIPReturning("127.0.0.1\n"),
	}

	for name, get := range unusableResponses {
		t.Run(name, func(t *testing.T) {
			cidr, err := DetectPublicIP(get)

			if err == nil {
				t.Fatalf("DetectPublicIP() returned %q and a nil error, want a failure", cidr)
			}
			if cidr != "" {
				t.Errorf("DetectPublicIP() = %q on failure, want the empty string", cidr)
			}
		})
	}
}

func TestPersistIngressCIDRRejectsOpenCIDR(t *testing.T) {
	base := t.TempDir()

	err := PersistIngressCIDR(base, "0.0.0.0/0")

	if err == nil {
		t.Fatal("PersistIngressCIDR() accepted 0.0.0.0/0")
	}
	if fileExists(TfvarsPath(base)) {
		t.Errorf("PersistIngressCIDR() wrote %s despite rejecting the value", TfvarsPath(base))
	}
}

func TestResolveIngressCIDR_FailureIsFatalOnCreate(t *testing.T) {
	base := t.TempDir()
	if err := PersistIngressCIDR(base, "198.51.100.4/32"); err != nil {
		t.Fatalf("persisting the ingress CIDR: %v", err)
	}

	cidr, warning, err := ResolveIngressCIDR(base, OpCreate, failingCheckIP("dial tcp: no route to host"))

	if err == nil {
		t.Fatalf("ResolveIngressCIDR(OpCreate) = %q with a nil error, want the detection failure", cidr)
	}
	assertContainsAll(t, "the create failure", err.Error(), "dial tcp: no route to host")
	if cidr != "" {
		t.Errorf("ResolveIngressCIDR(OpCreate) = %q, want the empty string", cidr)
	}
	if warning != "" {
		t.Errorf("ResolveIngressCIDR(OpCreate) warned %q, want no warning on a fatal path", warning)
	}
}

func TestResolveIngressCIDR_FallsBackOnDestroy(t *testing.T) {
	base := t.TempDir()
	if err := PersistIngressCIDR(base, "198.51.100.4/32"); err != nil {
		t.Fatalf("persisting the ingress CIDR: %v", err)
	}

	cidr, warning, err := ResolveIngressCIDR(base, OpDestroy, failingCheckIP("dial tcp: no route to host"))

	if err != nil {
		t.Fatalf("ResolveIngressCIDR(OpDestroy) error = %v, want the persisted value", err)
	}
	if cidr != "198.51.100.4/32" {
		t.Errorf("ResolveIngressCIDR(OpDestroy) = %q, want the persisted %q", cidr, "198.51.100.4/32")
	}
	assertContainsAll(t, "the fallback warning", warning,
		"warning: public IP detection failed",
		"dial tcp: no route to host",
		"198.51.100.4/32",
	)
}

func TestResolveIngressCIDR_FailsOnDestroyWithNothingPersisted(t *testing.T) {
	base := t.TempDir()

	cidr, warning, err := ResolveIngressCIDR(base, OpDestroy, failingCheckIP("dial tcp: no route to host"))

	if err == nil {
		t.Fatalf("ResolveIngressCIDR(OpDestroy) = %q with a nil error, want a failure", cidr)
	}
	assertContainsAll(t, "the teardown failure", err.Error(),
		"dial tcp: no route to host",
		"no ingress CIDR was persisted",
	)
	if cidr != "" {
		t.Errorf("ResolveIngressCIDR(OpDestroy) = %q, want the empty string", cidr)
	}
	if warning != "" {
		t.Errorf("ResolveIngressCIDR(OpDestroy) warned %q, want no warning on a fatal path", warning)
	}
}

func TestResolveIngressCIDR_PersistsWhatItDetects(t *testing.T) {
	for _, op := range []Operation{OpCreate, OpDestroy} {
		t.Run(op.String(), func(t *testing.T) {
			base := t.TempDir()

			cidr, warning, err := ResolveIngressCIDR(base, op, fakeCheckIPReturning("203.0.113.7\n"))

			if err != nil {
				t.Fatalf("ResolveIngressCIDR(%s) error = %v", op, err)
			}
			if cidr != "203.0.113.7/32" {
				t.Errorf("ResolveIngressCIDR(%s) = %q, want %q", op, cidr, "203.0.113.7/32")
			}
			if warning != "" {
				t.Errorf("ResolveIngressCIDR(%s) warned %q, want no warning when detection succeeds", op, warning)
			}

			persisted, err := ReadPersistedIngressCIDR(base)
			if err != nil {
				t.Fatalf("ReadPersistedIngressCIDR() error = %v", err)
			}
			if persisted != "203.0.113.7/32" {
				t.Errorf("persisted CIDR = %q, want %q", persisted, "203.0.113.7/32")
			}
		})
	}
}

// TestResolveIngressCIDR_NeverYieldsAnOpenCIDR covers the whole failure surface
// at once, because an open CIDR reached by any of these paths would expose SSH
// to the internet.
func TestResolveIngressCIDR_NeverYieldsAnOpenCIDR(t *testing.T) {
	unusable := map[string]HTTPGetFunc{
		"transport failure": failingCheckIP("dial tcp: no route to host"),
		"not an address":    fakeCheckIPReturning("<html>captive portal</html>\n"),
		"already open":      fakeCheckIPReturning(openIngressCIDR + "\n"),
		"unspecified IPv4":  fakeCheckIPReturning("0.0.0.0\n"),
	}
	persistedValues := map[string]string{
		"nothing persisted":      "",
		"an open CIDR persisted": openIngressCIDR,
	}

	for _, op := range []Operation{OpCreate, OpDestroy} {
		for detection, get := range unusable {
			for state, persisted := range persistedValues {
				t.Run(op.String()+"/"+detection+"/"+state, func(t *testing.T) {
					base := t.TempDir()
					if persisted != "" {
						writeTfvars(t, base, persisted)
					}

					cidr, _, err := ResolveIngressCIDR(base, op, get)

					if err == nil {
						t.Fatalf("ResolveIngressCIDR() = %q with a nil error, want a failure", cidr)
					}
					if cidr != "" {
						t.Errorf("ResolveIngressCIDR() = %q on failure, want the empty string", cidr)
					}
				})
			}
		}
	}
}

// writeTfvars bypasses PersistIngressCIDR so a test can seed a value that
// PersistIngressCIDR itself would refuse to write.
func writeTfvars(t *testing.T, base, cidr string) {
	t.Helper()

	if err := os.MkdirAll(TerraformDir(base), scaffoldingDirMode); err != nil {
		t.Fatalf("creating %s: %v", TerraformDir(base), err)
	}
	if err := os.WriteFile(TfvarsPath(base), []byte("ingress_cidr = \""+cidr+"\"\n"), tfvarsFileMode); err != nil {
		t.Fatalf("writing %s: %v", TfvarsPath(base), err)
	}
}

func TestReadPersistedIngressCIDRReportsMissingFile(t *testing.T) {
	base := t.TempDir()

	cidr, err := ReadPersistedIngressCIDR(base)

	if err == nil {
		t.Fatalf("ReadPersistedIngressCIDR() = %q with a nil error, want a failure", cidr)
	}
	if cidr != "" {
		t.Errorf("ReadPersistedIngressCIDR() = %q, want the empty string", cidr)
	}
}
