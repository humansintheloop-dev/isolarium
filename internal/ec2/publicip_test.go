package ec2

import (
	"errors"
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
