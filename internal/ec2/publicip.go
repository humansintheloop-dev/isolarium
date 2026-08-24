package ec2

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	checkIPURL      = "https://checkip.amazonaws.com"
	checkIPTimeout  = 10 * time.Second
	tfvarsFileMode  = 0644
	openIngressCIDR = "0.0.0.0/0"
)

// HTTPGetFunc fetches a URL and returns its body, so tests can drive detection
// without network access.
type HTTPGetFunc func(url string) (string, error)

var persistedCIDRPattern = regexp.MustCompile(`ingress_cidr\s*=\s*"([^"]+)"`)

// DetectPublicIP returns the host's public address as a /32. It fails rather
// than ever yielding an open CIDR.
func DetectPublicIP(get HTTPGetFunc) (string, error) {
	body, err := get(checkIPURL)
	if err != nil {
		return "", fmt.Errorf("detecting public IP from %s: %w", checkIPURL, err)
	}

	address := strings.TrimSpace(body)
	ip := net.ParseIP(address)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("detecting public IP from %s: %q is not an IPv4 address", checkIPURL, address)
	}
	if !ip.IsGlobalUnicast() {
		return "", fmt.Errorf("detecting public IP from %s: %q is not a routable address", checkIPURL, address)
	}
	return address + "/32", nil
}

// Operation names which isolarium verb is resolving the ingress CIDR, because
// the policy when detection fails differs between launching an environment and
// tearing one down. The type is exported so the backend package can name the
// operation it is performing.
type Operation int

const (
	// OpCreate never falls back: an environment launched against a stale CIDR
	// would be unreachable, and the only other option is opening SSH up.
	OpCreate Operation = iota
	// OpDestroy falls back to the last known CIDR, so that being off the network
	// isolarium was created from never blocks a teardown.
	OpDestroy
	// OpConnect warns and proceeds with no CIDR at all: a run or shell whose
	// host address cannot be detected may still reach the instance, so being
	// offline from checkip never blocks a command that might work.
	OpConnect
)

func (o Operation) String() string {
	switch o {
	case OpDestroy:
		return "destroy"
	case OpConnect:
		return "connect"
	default:
		return "create"
	}
}

// ResolveIngressCIDR yields the CIDR SSH ingress is pinned to, together with a
// warning the caller is expected to report when it had to fall back. Detection
// failure is fatal for OpCreate, falls back to the persisted CIDR for
// OpDestroy, and yields an empty CIDR with a warning for OpConnect; no path
// returns an open CIDR. It never writes the persisted file: that records the
// CIDR last applied, which the caller knows only once its apply has succeeded.
func ResolveIngressCIDR(base string, op Operation, get HTTPGetFunc) (cidr string, warning string, err error) {
	cidr, err = DetectPublicIP(get)
	if err == nil {
		return cidr, "", nil
	}
	if op == OpConnect {
		return "", fmt.Sprintf("warning: public IP detection failed (%v); connecting without checking the SSH ingress rule", err), nil
	}
	if op != OpDestroy {
		return "", "", err
	}

	persisted, persistErr := ReadPersistedIngressCIDR(base)
	if persistErr != nil {
		return "", "", fmt.Errorf("%w; and no ingress CIDR was persisted: %v", err, persistErr)
	}
	return persisted, fallbackWarning(err, persisted), nil
}

func fallbackWarning(err error, persisted string) string {
	return fmt.Sprintf("warning: public IP detection failed (%v); falling back to the last known ingress CIDR %s", err, persisted)
}

func PersistIngressCIDR(base, cidr string) error {
	if err := rejectOpenIngress(cidr); err != nil {
		return err
	}
	if err := os.MkdirAll(TerraformDir(base), scaffoldingDirMode); err != nil {
		return fmt.Errorf("creating %s: %w", TerraformDir(base), err)
	}

	line := fmt.Sprintf("ingress_cidr = %q\n", cidr)
	if err := os.WriteFile(TfvarsPath(base), []byte(line), tfvarsFileMode); err != nil {
		return fmt.Errorf("writing %s: %w", TfvarsPath(base), err)
	}
	return nil
}

func ReadPersistedIngressCIDR(base string) (string, error) {
	data, err := os.ReadFile(TfvarsPath(base))
	if err != nil {
		return "", fmt.Errorf("reading the last known ingress CIDR from %s: %w", TfvarsPath(base), err)
	}

	match := persistedCIDRPattern.FindSubmatch(data)
	if match == nil {
		return "", fmt.Errorf("reading the last known ingress CIDR from %s: no ingress_cidr assignment", TfvarsPath(base))
	}
	cidr := string(match[1])
	if err := rejectOpenIngress(cidr); err != nil {
		return "", err
	}
	return cidr, nil
}

func rejectOpenIngress(cidr string) error {
	address, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("ingress CIDR %q is not a valid CIDR: %w", cidr, err)
	}
	if cidr == openIngressCIDR || !address.IsGlobalUnicast() {
		return fmt.Errorf("refusing ingress CIDR %q: SSH must be restricted to the host address", cidr)
	}
	if ones, bits := network.Mask.Size(); ones != bits {
		return fmt.Errorf("refusing ingress CIDR %q: SSH must be restricted to a single host address", cidr)
	}
	return nil
}

func DefaultHTTPGet(url string) (string, error) {
	client := &http.Client{Timeout: checkIPTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
