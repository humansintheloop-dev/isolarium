package ec2

import (
	"fmt"
	"regexp"
)

// NamePattern keeps an environment name simultaneously usable as a Terraform
// identifier, a filename component and an AWS tag value, so no sanitisation is
// ever needed downstream.
const NamePattern = `^[a-z][a-z0-9-]{0,31}$`

var nameMatcher = regexp.MustCompile(NamePattern)

func ValidateName(name string) error {
	if nameMatcher.MatchString(name) {
		return nil
	}
	return fmt.Errorf("invalid --name %q for --type ec2: must match %s", name, NamePattern)
}
