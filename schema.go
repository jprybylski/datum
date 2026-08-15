// Package datum exposes Datum's configuration contract for commands and integrations that need
// to inspect it without loading a user's configuration file.
package datum

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:generate go run ./internal/cmd/genschema

// ConfigSchema returns an independent copy of the exact JSON Schema shipped with Datum.
func ConfigSchema() []byte {
	return []byte(configSchema)
}

// SourceFieldSpec describes one source configuration field as declared by ConfigSchema.
type SourceFieldSpec struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// SourceTypeSpec describes one dataset source type as declared by ConfigSchema.
type SourceTypeSpec struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Fields      []SourceFieldSpec `json:"fields"`
}

type rawSchema struct {
	Definitions map[string]struct {
		Description string   `json:"description"`
		Required    []string `json:"required"`
		Properties  map[string]struct {
			Description string   `json:"description"`
			Enum        []string `json:"enum"`
		} `json:"properties"`
	} `json:"definitions"`
}

// SourceTypeSpecs derives the source-type reference from ConfigSchema, keeping the CLI and editor
// validation contract on one source of truth.
func SourceTypeSpecs() ([]SourceTypeSpec, error) {
	return sourceTypeSpecs([]byte(configSchema))
}

func sourceTypeSpecs(schemaJSON []byte) ([]SourceTypeSpec, error) {
	var schema rawSchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil, fmt.Errorf("parse embedded configuration schema: %w", err)
	}

	specs := make([]SourceTypeSpec, 0, len(schema.Definitions))
	for _, definition := range schema.Definitions {
		typeProperty, ok := definition.Properties["type"]
		if !ok || len(typeProperty.Enum) != 1 {
			continue
		}
		required := make(map[string]bool, len(definition.Required))
		for _, name := range definition.Required {
			required[name] = true
		}
		fieldNames := make([]string, 0, len(definition.Properties))
		for name := range definition.Properties {
			fieldNames = append(fieldNames, name)
		}
		sort.Slice(fieldNames, func(i, j int) bool {
			return fieldRank(fieldNames[i]) < fieldRank(fieldNames[j])
		})

		spec := SourceTypeSpec{Type: typeProperty.Enum[0], Description: definition.Description}
		for _, name := range fieldNames {
			property := definition.Properties[name]
			description := property.Description
			if name == "type" {
				description = fmt.Sprintf("Must be %q.", spec.Type)
			}
			spec.Fields = append(spec.Fields, SourceFieldSpec{
				Name: name, Required: required[name], Description: description,
			})
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Type < specs[j].Type })
	return specs, nil
}

func fieldRank(name string) string {
	order := []string{"type", "url", "path", "ref", "fingerprint_cmd", "fetch_cmd"}
	for i, candidate := range order {
		if name == candidate {
			return fmt.Sprintf("%02d", i)
		}
	}
	return "99" + strings.ToLower(name)
}
