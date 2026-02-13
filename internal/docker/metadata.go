package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type DockerMetadata struct {
	Type          string    `json:"type"`
	WorkDirectory string    `json:"work_directory"`
	CreatedAt     time.Time `json:"created_at"`
}

type MetadataStore struct {
	baseDir string
	name    string
}

func NewMetadataStore(baseDir, name string) *MetadataStore {
	return &MetadataStore{baseDir: baseDir, name: name}
}

func (s *MetadataStore) dir() string {
	return filepath.Join(s.baseDir, s.name, "container")
}

func (s *MetadataStore) path() string {
	return filepath.Join(s.dir(), "metadata.json")
}

func (s *MetadataStore) Write(envType, workDirectory string) error {
	meta := DockerMetadata{
		Type:          envType,
		WorkDirectory: workDirectory,
		CreatedAt:     time.Now().UTC(),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.MkdirAll(s.dir(), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	if err := os.WriteFile(s.path(), data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

func (s *MetadataStore) Read() (*DockerMetadata, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta DockerMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &meta, nil
}

func (s *MetadataStore) Cleanup() error {
	if err := os.RemoveAll(s.dir()); err != nil {
		return fmt.Errorf("failed to cleanup metadata: %w", err)
	}
	return nil
}
