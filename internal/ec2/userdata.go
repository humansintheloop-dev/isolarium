package ec2

import (
	_ "embed"
	"fmt"
)

//go:embed cloud-init.yaml
var cloudInit string

// UserDataLimit is the largest user_data document EC2 accepts, in bytes.
const UserDataLimit = 16384

// RenderUserData returns the cloud-init document handed to EC2 as user_data. It
// provisions the same toolchain as internal/lima/template.yaml, plus tmux.
func RenderUserData() string {
	return cloudInit
}

// ValidateUserDataSize rejects a document EC2 would refuse at RunInstances, so a
// toolchain that has outgrown the limit is caught on the host rather than
// halfway through an apply.
func ValidateUserDataSize(doc string) error {
	if len(doc) > UserDataLimit {
		return fmt.Errorf("rendered user_data is %d bytes, exceeding the EC2 limit of %d bytes",
			len(doc), UserDataLimit)
	}
	return nil
}
