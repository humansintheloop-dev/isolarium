package nono

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"
)

//go:embed isolarium-nono-profile.json
var embeddedProfile []byte

var (
	profilePath string
	profileOnce sync.Once
)

func getProfilePath() string {
	profileOnce.Do(func() {
		dir, err := os.MkdirTemp("", "isolarium-profile-*")
		if err != nil {
			panic("failed to create temp dir for nono profile: " + err.Error())
		}
		if err := os.Chmod(dir, 0700); err != nil {
			panic("failed to secure nono profile dir: " + err.Error())
		}
		p := filepath.Join(dir, "isolarium-nono-profile.json")
		if err := os.WriteFile(p, embeddedProfile, 0400); err != nil {
			panic("failed to write nono profile: " + err.Error())
		}
		profilePath = p
	})
	return profilePath
}
