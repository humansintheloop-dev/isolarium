package lima

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RepoMetadata contains information about the cloned repository
type RepoMetadata struct {
	Owner    string    `json:"owner"`
	Repo     string    `json:"repo"`
	Branch   string    `json:"branch"`
	ClonedAt time.Time `json:"cloned_at"`
}

// MetadataStore reads and writes repository metadata on the host filesystem.
// Metadata is stored at <baseDir>/<vmName>/repo.json.
type MetadataStore struct {
	baseDir string
	vmName  string
}

func NewMetadataStore(baseDir, vmName string) *MetadataStore {
	return &MetadataStore{baseDir: baseDir, vmName: vmName}
}

func (s *MetadataStore) dir() string {
	return filepath.Join(s.baseDir, s.vmName)
}

func (s *MetadataStore) path() string {
	return filepath.Join(s.dir(), "repo.json")
}

func (s *MetadataStore) Write(owner, repo, branch string) error {
	meta := RepoMetadata{
		Owner:    owner,
		Repo:     repo,
		Branch:   branch,
		ClonedAt: time.Now().UTC(),
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

func (s *MetadataStore) Read() (*RepoMetadata, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta RepoMetadata
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

// defaultStore returns a MetadataStore using ~/.isolarium/ as the base directory
func defaultStore() *MetadataStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return NewMetadataStore(filepath.Join(home, ".isolarium"), vmName)
}

// WriteRepoMetadata writes repository metadata to the host filesystem
func WriteRepoMetadata(owner, repo, branch string) error {
	return defaultStore().Write(owner, repo, branch)
}

// ReadRepoMetadata reads repository metadata from the host filesystem
func ReadRepoMetadata() (*RepoMetadata, error) {
	return defaultStore().Read()
}

// CleanupHostMetadata removes the host-side metadata directory for the VM
func CleanupHostMetadata() error {
	return defaultStore().Cleanup()
}
