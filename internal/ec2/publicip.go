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
