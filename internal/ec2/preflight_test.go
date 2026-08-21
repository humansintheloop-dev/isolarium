package ec2

import "testing"

func lookupEnvReturning(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestRequireRegion_ReturnsAWSRegion(t *testing.T) {
	region, err := RequireRegion(lookupEnvReturning(map[string]string{"AWS_REGION": "us-west-2"}))
	if err != nil {
		t.Fatalf("RequireRegion() returned error: %v", err)
	}
	if region != "us-west-2" {
		t.Errorf("RequireRegion() = %q, want %q", region, "us-west-2")
	}
}

const missingRegionMessage = "AWS_REGION is required for --type ec2; set it in .env.local"

func TestRequireRegion_ErrorsWhenUnset(t *testing.T) {
	_, err := RequireRegion(lookupEnvReturning(map[string]string{}))
	if err == nil {
		t.Fatal("RequireRegion() returned nil error when AWS_REGION is unset")
	}
	if err.Error() != missingRegionMessage {
		t.Errorf("RequireRegion() error = %q, want %q", err.Error(), missingRegionMessage)
	}
}

func TestRequireRegion_ErrorsWhenEmpty(t *testing.T) {
	_, err := RequireRegion(lookupEnvReturning(map[string]string{"AWS_REGION": ""}))
	if err == nil {
		t.Fatal("RequireRegion() returned nil error when AWS_REGION is empty")
	}
	if err.Error() != missingRegionMessage {
		t.Errorf("RequireRegion() error = %q, want %q", err.Error(), missingRegionMessage)
	}
}
