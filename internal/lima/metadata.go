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
	return filepath.Join(s.baseDir, s.vmName, "vm")
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

func storeFor(name string) *MetadataStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return NewMetadataStore(filepath.Join(home, ".isolarium"), name)
}

func WriteRepoMetadata(name, owner, repo, branch string) error {
	return storeFor(name).Write(owner, repo, branch)
}

func ReadRepoMetadata(name string) (*RepoMetadata, error) {
	return storeFor(name).Read()
}

func CleanupHostMetadata(name string) error {
	return storeFor(name).Cleanup()
}
