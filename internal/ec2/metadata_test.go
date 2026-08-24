package ec2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const terraformOutputJSON = `{
  "instance_id_my-work": {"sensitive": false, "type": "string", "value": "i-0123456789abcdef0"},
  "public_dns_my-work":  {"sensitive": false, "type": "string", "value": "ec2-203-0-113-7.compute-1.amazonaws.com"}
}`

func sampleMetadata() Metadata {
	return Metadata{
		InstanceID: "i-0123456789abcdef0",
		PublicDNS:  "ec2-203-0-113-7.compute-1.amazonaws.com",
		Region:     testRegion,
		Owner:      "humansintheloop-dev",
		Repo:       "isolarium",
		Branch:     "main",
		CreatedAt:  time.Date(2026, 8, 19, 14, 3, 21, 0, time.UTC),
	}
}

func TestMetadataStore_WriteThenRead_RoundTripsEveryField(t *testing.T) {
	base := t.TempDir()
	store := NewMetadataStore(base, "my-work")
	want := sampleMetadata()

	if err := store.Write(want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := store.Read()

	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if *got != want {
		t.Errorf("Read() = %+v, want %+v", *got, want)
	}
}

func TestMetadataStore_Write_LandsUnderTheEnvironmentDirectory(t *testing.T) {
	base := t.TempDir()
	store := NewMetadataStore(base, "my-work")

	if err := store.Write(sampleMetadata()); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	want := filepath.Join(base, "my-work", "ec2", "metadata.json")
	if store.Path() != want {
		t.Errorf("Path() = %q, want %q", store.Path(), want)
	}
	assertMetadataFileFields(t, want,
		`"instance_id": "i-0123456789abcdef0"`,
		`"public_dns": "ec2-203-0-113-7.compute-1.amazonaws.com"`,
		`"region": "us-west-2"`,
		`"created_at": "2026-08-19T14:03:21Z"`,
	)
}

func TestMetadataStore_Read_FailsWhenTheEnvironmentIsUnknown(t *testing.T) {
	_, err := NewMetadataStore(t.TempDir(), "my-work").Read()

	if err == nil {
		t.Fatal("Read() returned nil error for an environment that was never created")
	}
}

func TestMetadataStore_Cleanup_RemovesTheEnvironmentDirectoryAndToleratesItsAbsence(t *testing.T) {
	base := t.TempDir()
	store := NewMetadataStore(base, "my-work")
	if err := store.Write(sampleMetadata()); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := store.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "my-work", "ec2")); !os.IsNotExist(err) {
		t.Errorf("the environment directory survived Cleanup()")
	}

	if err := store.Cleanup(); err != nil {
		t.Errorf("Cleanup() on an absent directory error = %v, want nil", err)
	}
}

func TestParseTerraformOutput_ExtractsTheNamedInstanceOutputs(t *testing.T) {
	instanceID, publicDNS, err := ParseTerraformOutput([]byte(terraformOutputJSON), "my-work")

	if err != nil {
		t.Fatalf("ParseTerraformOutput() error = %v", err)
	}
	if instanceID != "i-0123456789abcdef0" {
		t.Errorf("instanceID = %q, want %q", instanceID, "i-0123456789abcdef0")
	}
	if publicDNS != "ec2-203-0-113-7.compute-1.amazonaws.com" {
		t.Errorf("publicDNS = %q, want %q", publicDNS, "ec2-203-0-113-7.compute-1.amazonaws.com")
	}
}

func TestParseTerraformOutput_FailsWhenTheInstanceOutputsAreAbsent(t *testing.T) {
	_, _, err := ParseTerraformOutput([]byte(terraformOutputJSON), "other")

	if err == nil {
		t.Fatal("ParseTerraformOutput() returned nil error for an unknown environment")
	}
	if !strings.Contains(err.Error(), "instance_id_other") {
		t.Errorf("error = %q, want it to name the missing output", err.Error())
	}
}

func TestParseTerraformOutput_FailsWhenTheDocumentIsNotJSON(t *testing.T) {
	_, _, err := ParseTerraformOutput([]byte("Error: no outputs found"), "my-work")

	if err == nil {
		t.Fatal("ParseTerraformOutput() returned nil error for a non-JSON document")
	}
}

func assertMetadataFileFields(t *testing.T, path string, wanted ...string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	assertContainsAll(t, "metadata.json", string(data), wanted...)
}
