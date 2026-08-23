package ec2

import "testing"

func TestIngressChanged_ComparesTheDetectedCIDRWithThePersistedOne(t *testing.T) {
	cases := map[string]struct {
		persisted string // empty when no apply has recorded an address yet
		want      IngressChange
	}{
		"unchanged when the host is where the rule was applied": {
			persisted: "203.0.113.7/32",
			want:      IngressChange{Detected: "203.0.113.7/32", Persisted: "203.0.113.7/32", Changed: false},
		},
		"changed when the host moved": {
			persisted: "198.51.100.4/32",
			want:      IngressChange{Detected: "203.0.113.7/32", Persisted: "198.51.100.4/32", Changed: true},
		},
		"changed when nothing was ever recorded": {
			persisted: "",
			want:      IngressChange{Detected: "203.0.113.7/32", Persisted: "", Changed: true},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			if tc.persisted != "" {
				writeTfvars(t, base, tc.persisted)
			}

			change, warning, err := IngressChanged(base, fakeCheckIPReturning("203.0.113.7\n"))

			if err != nil {
				t.Fatalf("IngressChanged() error = %v", err)
			}
			if warning != "" {
				t.Errorf("IngressChanged() warned %q, want no warning when detection succeeds", warning)
			}
			if change != tc.want {
				t.Errorf("IngressChanged() = %+v, want %+v", change, tc.want)
			}
		})
	}
}

func TestIngressChanged_WarnsWithoutAChangeWhenDetectionFails(t *testing.T) {
	base := t.TempDir()
	writeTfvars(t, base, "198.51.100.4/32")

	change, warning, err := IngressChanged(base, failingCheckIP("dial tcp: no route to host"))

	if err != nil {
		t.Fatalf("IngressChanged() error = %v, want detection failure to be a warning", err)
	}
	if change.Changed {
		t.Errorf("IngressChanged() = %+v, want no change reported when the host address is unknown", change)
	}
	assertContainsAll(t, "the connect warning", warning, "warning: public IP detection failed", "dial tcp: no route to host")
}
