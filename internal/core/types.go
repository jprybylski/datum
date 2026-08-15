package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jprybylski/datum"
	"github.com/jprybylski/datum/internal/registry"
)

// Types prints the source types available in this build. With no names it prints a compact list;
// with names it prints the complete configuration fields for those types.
func Types(names []string) int {
	return types(names, os.Stdout, datum.SourceTypeSpecs, registry.Names, func(name string) bool {
		_, ok := registry.Get(name)
		return ok
	})
}

func types(
	names []string,
	out io.Writer,
	loadSpecs func() ([]datum.SourceTypeSpec, error),
	availableNames func() []string,
	isRegistered func(string) bool,
) int {
	allSpecs, err := loadSpecs()
	if err != nil {
		printTypesError(out, err.Error())
		return 1
	}
	byName := make(map[string]datum.SourceTypeSpec, len(allSpecs))
	for _, spec := range allSpecs {
		byName[spec.Type] = spec
	}

	available := availableNames()
	specs := make([]datum.SourceTypeSpec, 0, len(available))
	for _, name := range available {
		spec, ok := byName[name]
		if !ok {
			printTypesError(out, fmt.Sprintf("registered source type %q is missing from the configuration schema", name))
			return 1
		}
		specs = append(specs, spec)
	}
	if len(names) > 0 {
		specs = make([]datum.SourceTypeSpec, 0, len(names))
		for _, name := range names {
			spec, documented := byName[name]
			if !documented || !isRegistered(name) {
				printTypesError(out, fmt.Sprintf("unknown dataset source type %q", name))
				return 2
			}
			specs = append(specs, spec)
		}
	}

	if JSONOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		err := encoder.Encode(struct {
			Types []datum.SourceTypeSpec `json:"types"`
		}{Types: specs})
		if err != nil {
			fmt.Fprintf(out, "json encode error: %v\n", err)
			return 1
		}
		return 0
	}

	if len(names) == 0 {
		fmt.Fprintln(out, colorize(ansiCyan, "Available dataset source types:"))
		for _, spec := range specs {
			fmt.Fprintf(out, "  %-10s %s\n", spec.Type, spec.Description)
		}
		fmt.Fprintln(out, "\nRun `datum types TYPE [TYPE ...]` for configuration details.")
		return 0
	}

	for i, spec := range specs {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, colorize(ansiCyan, spec.Type))
		fmt.Fprintln(out, "  "+spec.Description)
		fmt.Fprintln(out, "  Fields:")
		for _, field := range spec.Fields {
			requirement := "optional"
			if field.Required {
				requirement = "required"
			}
			fmt.Fprintf(out, "    %-17s (%s) %s\n", field.Name, requirement, field.Description)
		}
	}
	return 0
}

func printTypesError(out io.Writer, message string) {
	if JSONOutput {
		err := json.NewEncoder(out).Encode(struct {
			Error string `json:"error"`
		}{Error: message})
		if err == nil {
			return
		}
	}
	fmt.Fprintln(out, "types error:", message)
}
