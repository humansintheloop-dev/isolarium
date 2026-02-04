package lima

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const metadataDir = ".isolarium"
const metadataPath = ".isolarium/repo.json"

// RepoMetadata contains information about the cloned repository
type RepoMetadata struct {
	Owner    string    `json:"owner"`
	Repo     string    `json:"repo"`
	Branch   string    `json:"branch"`
	ClonedAt time.Time `json:"cloned_at"`
}

// BuildWriteMetadataCommand constructs the command to write metadata in the VM
func BuildWriteMetadataCommand(jsonData string) []string {
	script := fmt.Sprintf("mkdir -p ~/%s && cat > ~/%s << 'EOF'\n%s\nEOF", metadataDir, metadataPath, jsonData)
	return []string{"limactl", "shell", vmName, "--", "bash", "-c", script}
}

// BuildReadMetadataCommand constructs the command to read metadata from the VM
func BuildReadMetadataCommand() []string {
	return []string{"limactl", "shell", vmName, "--", "cat", metadataPath}
}

// WriteRepoMetadata writes repository metadata to the VM
func WriteRepoMetadata(owner, repo, branch string) error {
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

	args := BuildWriteMetadataCommand(string(data))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// ReadRepoMetadata reads repository metadata from the VM
func ReadRepoMetadata() (*RepoMetadata, error) {
	args := BuildReadMetadataCommand()
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta RepoMetadata
	if err := json.Unmarshal(output, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &meta, nil
}
