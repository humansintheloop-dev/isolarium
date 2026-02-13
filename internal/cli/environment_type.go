package cli

import "fmt"

type environmentType string

func (e *environmentType) String() string {
	return string(*e)
}

func (e *environmentType) Set(val string) error {
	switch val {
	case "vm", "container":
		*e = environmentType(val)
		return nil
	default:
		return fmt.Errorf("invalid type %q: must be \"vm\" or \"container\"", val)
	}
}

func (e *environmentType) Type() string {
	return "string"
}
