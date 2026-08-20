package ec2

import _ "embed"

//go:embed cloud-init.yaml
var cloudInit string

// RenderUserData returns the cloud-init document handed to EC2 as user_data. It
// provisions the same toolchain as internal/lima/template.yaml, plus tmux.
func RenderUserData() string {
	return cloudInit
}
