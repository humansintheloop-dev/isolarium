// Package toolchain holds provisioning scripts every backend runs inside its
// environment, so that Lima and EC2 install the same toolchain from one source.
package toolchain

import _ "embed"

// InstallUsingSDKMANScript installs Java and Gradle through the SDKMAN that
// the environment's first-boot provisioning already put in place. It is fed to
// `bash -s` on the environment's standard input.
//
//go:embed install-using-sdkman.sh
var InstallUsingSDKMANScript string
