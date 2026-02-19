package nono

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type NonoMetadata struct {
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

func (s *MetadataStore) Dir() string {
	return filepath.Join(s.baseDir, s.name, "nono")
}

func (s *MetadataStore) path() string {
	return filepath.Join(s.Dir(), "metadata.json")
}

func (s *MetadataStore) Write(envType, workDirectory string) error {
	meta := NonoMetadata{
		Type:          envType,
		WorkDirectory: workDirectory,
		CreatedAt:     time.Now().UTC(),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.MkdirAll(s.Dir(), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	if err := os.WriteFile(s.path(), data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

func (s *MetadataStore) Read() (*NonoMetadata, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta NonoMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &meta, nil
}

func (s *MetadataStore) Cleanup() error {
	if err := os.RemoveAll(s.Dir()); err != nil {
		return fmt.Errorf("failed to cleanup metadata: %w", err)
	}
	return nil
}
