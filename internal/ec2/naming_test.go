package ec2

import (
	"strings"
	"testing"
)

func TestEC2ValidateNameAcceptsATerraformAndFilenameSafeName(t *testing.T) {
	tests := []struct {
		description string
		name        string
	}{
		{description: "a single letter", name: "a"},
		{description: "letters and a hyphen", name: "my-work"},
		{description: "digits after the first letter", name: "env2"},
		{description: "the longest allowed name", name: nameOfLength(32)},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			if err := ValidateName(tc.name); err != nil {
				t.Errorf("ValidateName(%q) = %v, want nil", tc.name, err)
			}
		})
	}
}

func TestEC2ValidateNameRejectsANameThatWouldBreakTerraformOrFilenames(t *testing.T) {
	tests := []struct {
		description string
		name        string
	}{
		{description: "an upper case letter and an underscore", name: "My_Env"},
		{description: "a leading digit", name: "1abc"},
		{description: "a leading hyphen", name: "-abc"},
		{description: "empty", name: ""},
		{description: "one character too long", name: nameOfLength(33)},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidateName(tc.name)

			if err == nil {
				t.Fatalf("ValidateName(%q) = nil, want an error", tc.name)
			}
			assertContainsAll(t, "the rejection message", err.Error(),
				"invalid --name", "for --type ec2", NamePattern)
		})
	}
}

func TestEC2ValidateNameStatesTheOffendingName(t *testing.T) {
	err := ValidateName("My_Env")

	if err == nil {
		t.Fatal("ValidateName(\"My_Env\") = nil, want an error")
	}
	want := `invalid --name "My_Env" for --type ec2: must match ` + NamePattern
	if err.Error() != want {
		t.Errorf("ValidateName(\"My_Env\") = %q, want %q", err.Error(), want)
	}
}

func nameOfLength(length int) string {
	return "a" + strings.Repeat("b", length-1)
}
