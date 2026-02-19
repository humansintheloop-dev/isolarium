package cli

import "fmt"

type environmentType string

func (e *environmentType) String() string {
	return string(*e)
}

func (e *environmentType) Set(val string) error {
	switch val {
	case "vm", "container", "nono":
		*e = environmentType(val)
		return nil
	default:
		return fmt.Errorf("invalid type %q: must be \"vm\", \"container\", or \"nono\"", val)
	}
}

func (e *environmentType) Type() string {
	return "string"
}
