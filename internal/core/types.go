package core

import (
	"encoding/json"
	"fmt"

	"github.com/jprybylski/datum"
	"github.com/jprybylski/datum/internal/registry"
)

// Types prints the source types available in this build. With no names it prints a compact list;
// with names it prints the complete configuration fields for those types.
func Types(names []string) int {
	allSpecs, err := datum.SourceTypeSpecs()
	if err != nil {
		printTypesError(err.Error())
		return 1
	}
	byName := make(map[string]datum.SourceTypeSpec, len(allSpecs))
	for _, spec := range allSpecs {
		byName[spec.Type] = spec
	}

	available := registry.Names()
	specs := make([]datum.SourceTypeSpec, 0, len(available))
	for _, name := range available {
		spec, ok := byName[name]
		if !ok {
			printTypesError(fmt.Sprintf("registered source type %q is missing from the configuration schema", name))
			return 1
		}
		specs = append(specs, spec)
	}
	if len(names) > 0 {
		specs = make([]datum.SourceTypeSpec, 0, len(names))
		for _, name := range names {
			spec, documented := byName[name]
			_, registered := registry.Get(name)
			if !documented || !registered {
				printTypesError(fmt.Sprintf("unknown dataset source type %q", name))
				return 2
			}
			specs = append(specs, spec)
		}
	}

	if JSONOutput {
		data, err := json.MarshalIndent(struct {
			Types []datum.SourceTypeSpec `json:"types"`
		}{Types: specs}, "", "  ")
		if err != nil {
			fmt.Printf("json encode error: %v\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	if len(names) == 0 {
		fmt.Println(colorize(ansiCyan, "Available dataset source types:"))
		for _, spec := range specs {
			fmt.Printf("  %-10s %s\n", spec.Type, spec.Description)
		}
		fmt.Println("\nRun `datum types TYPE [TYPE ...]` for configuration details.")
		return 0
	}

	for i, spec := range specs {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(colorize(ansiCyan, spec.Type))
		fmt.Println("  " + spec.Description)
		fmt.Println("  Fields:")
		for _, field := range spec.Fields {
			requirement := "optional"
			if field.Required {
				requirement = "required"
			}
			fmt.Printf("    %-17s (%s) %s\n", field.Name, requirement, field.Description)
		}
	}
	return 0
}

func printTypesError(message string) {
	if JSONOutput {
		data, err := json.Marshal(struct {
			Error string `json:"error"`
		}{Error: message})
		if err == nil {
			fmt.Println(string(data))
			return
		}
	}
	fmt.Println("types error:", message)
}
