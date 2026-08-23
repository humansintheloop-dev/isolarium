package ec2

// IngressChange is what a connecting operation learns about the host's
// address: where it is now, where SSH ingress was last applied to, and whether
// the two differ. Persisted is empty when no apply has recorded an address yet.
type IngressChange struct {
	Detected  string
	Persisted string
	Changed   bool
}

// IngressChanged compares the host's current public address with the CIDR SSH
// ingress was last applied with, under the OpConnect policy: a detection
// failure is returned as a warning with no change, so the caller can report it
// and connect anyway. A missing persisted file counts as changed, because no
// apply has ever recorded the host's address.
func IngressChanged(base string, get HTTPGetFunc) (change IngressChange, warning string, err error) {
	detected, warning, err := ResolveIngressCIDR(base, OpConnect, get)
	if err != nil || warning != "" {
		return IngressChange{}, warning, err
	}

	persisted, readErr := ReadPersistedIngressCIDR(base)
	if readErr != nil {
		return IngressChange{Detected: detected, Changed: true}, "", nil
	}
	return IngressChange{Detected: detected, Persisted: persisted, Changed: detected != persisted}, "", nil
}
