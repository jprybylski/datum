package core

import (
	"fmt"
	"io"
	"os"

	"github.com/jprybylski/datum"
)

// Schema writes the exact JSON Schema shipped with this build.
func Schema(out io.Writer) int {
	if _, err := out.Write(datum.ConfigSchema()); err != nil {
		fmt.Fprintf(os.Stderr, "schema output error: %v\n", err)
		return 1
	}
	return 0
}
