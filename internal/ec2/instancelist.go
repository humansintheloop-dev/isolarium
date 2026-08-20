package ec2

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ListInstanceNames reports the environments the Terraform working directory
// currently describes, and reports none when it has never been extracted.
func ListInstanceNames(base string) ([]string, error) {
	entries, err := os.ReadDir(TerraformDir(base))
	if err != nil {
		return nil, ignoreMissingTerraformDir(base, err)
	}
	return sortedInstanceNames(entries), nil
}

func ignoreMissingTerraformDir(base string, err error) error {
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("listing %s: %w", TerraformDir(base), err)
}

func sortedInstanceNames(entries []os.DirEntry) []string {
	var names []string
	for _, entry := range entries {
		if name, ok := instanceNameOf(entry.Name()); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func instanceNameOf(fileName string) (string, bool) {
	if !strings.HasPrefix(fileName, instanceFilePrefix) || !strings.HasSuffix(fileName, instanceFileSuffix) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(fileName, instanceFilePrefix), instanceFileSuffix), true
}
