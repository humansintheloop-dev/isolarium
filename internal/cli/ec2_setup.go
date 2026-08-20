package cli

import (
	"io"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func destroyEC2(out io.Writer, name string) error {
	b := backend.NewEC2Backend()
	b.Out = out
	return b.Destroy(name)
}
