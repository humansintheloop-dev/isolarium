package ec2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	metadataFileName = "metadata.json"
	metadataFileMode = 0644
	metadataDirMode  = 0755
)

// Metadata is everything run, shell, and status need about an environment
// without reaching for AWS credentials or the terraform binary.
type Metadata struct {
	InstanceID string    `json:"instance_id"`
	PublicDNS  string    `json:"public_dns"`
	Region     string    `json:"region"`
	Owner      string    `json:"owner"`
	Repo       string    `json:"repo"`
	Branch     string    `json:"branch"`
	CreatedAt  time.Time `json:"created_at"`
}

// MetadataStore reads and writes one environment's metadata at
// <baseDir>/<name>/ec2/metadata.json.
type MetadataStore struct {
	baseDir string
	name    string
}

func NewMetadataStore(baseDir, name string) *MetadataStore {
	return &MetadataStore{baseDir: baseDir, name: name}
}

func (s *MetadataStore) Dir() string {
	return filepath.Join(s.baseDir, s.name, "ec2")
}

func (s *MetadataStore) Path() string {
	return filepath.Join(s.Dir(), metadataFileName)
}

func (s *MetadataStore) Write(meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding metadata for %s: %w", s.name, err)
	}
	if err := os.MkdirAll(s.Dir(), metadataDirMode); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir(), err)
	}
	if err := os.WriteFile(s.Path(), data, metadataFileMode); err != nil {
		return fmt.Errorf("writing %s: %w", s.Path(), err)
	}
	return nil
}

func (s *MetadataStore) Read() (*Metadata, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.Path(), err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.Path(), err)
	}
	return &meta, nil
}

func (s *MetadataStore) Cleanup() error {
	if err := os.RemoveAll(s.Dir()); err != nil {
		return fmt.Errorf("removing %s: %w", s.Dir(), err)
	}
	return nil
}

type terraformOutputValue struct {
	Value string `json:"value"`
}

// ParseTerraformOutput lifts the two per-environment outputs declared in
// instance-<name>.tf out of a `terraform output -json` document.
func ParseTerraformOutput(data []byte, name string) (instanceID, publicDNS string, err error) {
	var outputs map[string]terraformOutputValue
	if err := json.Unmarshal(data, &outputs); err != nil {
		return "", "", fmt.Errorf("parsing terraform output for %s: %w", name, err)
	}

	instanceID, err = requiredOutput(outputs, "instance_id_"+name)
	if err != nil {
		return "", "", err
	}
	publicDNS, err = requiredOutput(outputs, "public_dns_"+name)
	if err != nil {
		return "", "", err
	}
	return instanceID, publicDNS, nil
}

func requiredOutput(outputs map[string]terraformOutputValue, key string) (string, error) {
	output, ok := outputs[key]
	if !ok || output.Value == "" {
		return "", fmt.Errorf("terraform output %s is missing or empty", key)
	}
	return output.Value, nil
}
