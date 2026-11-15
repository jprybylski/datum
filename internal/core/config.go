// Package core implements the main business logic for datum.
//
// This package contains the Check and Fetch operations, configuration parsing,
// lockfile management, and file hashing utilities.
//
// Key components:
//   - config.go: Configuration file structure and parsing
//   - engine.go: Check and Fetch implementation
//   - lock.go: Lockfile structure and I/O
//   - hash.go: File hashing utilities
package core

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jprybylski/datum/internal/registry"
)

// Config represents the structure of the .data.yaml configuration file.
//
// The configuration defines which external data sources to track and how to handle
// changes. It's typically version-controlled alongside your code.
//
// Go learning note: Struct tags (like `yaml:"version"`) tell the YAML library
// how to map between YAML field names and Go struct fields.
type Config struct {
	Version  int       `yaml:"version"`  // Config file format version (currently 1)
	Defaults Defaults  `yaml:"defaults"` // Default settings for all datasets
	Datasets []Dataset `yaml:"datasets"` // List of data sources to track
}

// Defaults specifies default settings that apply to all datasets unless overridden.
//
// This avoids repetition in the configuration file - common settings can be
// specified once and overridden per-dataset as needed.
type Defaults struct {
	Policy string `yaml:"policy"` // Default policy: "fail", "update", or "log"
	Algo   string `yaml:"algo"`   // Hash algorithm (currently only "sha256" is supported)
	Ignore bool   `yaml:"ignore"` // Whether fetched targets should be ignored by a detected VCS
}

// Dataset represents a single external data source to track.
//
// Each dataset has:
//   - Identification (ID, description)
//   - Source information (where to get the data)
//   - Target location (where to save it locally)
//   - Optional policy override
//
// Source Configuration:
//   - Use "source" for a single data source (backward compatible)
//   - Use "sources" for multiple data sources with fallback support
//   - Only one of "source" or "sources" should be specified
//
// When multiple sources are specified, they are tried in order. If one fails,
// the next source is attempted. The final policy judgment is applied only after
// all sources have been tried.
type Dataset struct {
	ID      string            `yaml:"id"`                // Unique identifier for this dataset
	Desc    string            `yaml:"desc"`              // Human-readable description
	Target  string            `yaml:"target"`            // Local file path where data will be saved
	Policy  string            `yaml:"policy,omitempty"`  // Policy override (empty uses default)
	Ignore  *bool             `yaml:"ignore,omitempty"`  // VCS-ignore override (nil uses default)
	Source  registry.Source   `yaml:"source,omitempty"`  // Single data source (backward compatible)
	Sources []registry.Source `yaml:"sources,omitempty"` // Multiple data sources with fallback
}

// ShouldIgnore returns the dataset-level ignore setting when present, otherwise the configured
// global default.
func (ds *Dataset) ShouldIgnore(defaultValue bool) bool {
	if ds.Ignore != nil {
		return *ds.Ignore
	}
	return defaultValue
}

// expandEnvInYAML replaces ${NAME} references in YAML scalar values. Mapping keys are left
// untouched so environment input cannot rename configuration fields. A doubled dollar sign
// escapes a reference: $${NAME} becomes the literal ${NAME}. When lenient is set, a reference
// to an unset variable is kept as written instead of failing (see expandScope).
//
// Every substituted value is remembered so output can redact it (see redact).
func expandEnvInYAML(node *yaml.Node, lenient bool) error {
	if node.Kind == yaml.MappingNode {
		for i := 1; i < len(node.Content); i += 2 {
			if err := expandEnvInYAML(node.Content[i], lenient); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		expanded, err := expandEnvValue(node.Value, lenient)
		if err != nil {
			return fmt.Errorf("line %d, column %d: %w", node.Line, node.Column, err)
		}
		node.Value = expanded
		return nil
	}
	for _, child := range node.Content {
		if err := expandEnvInYAML(child, lenient); err != nil {
			return err
		}
	}
	return nil
}

func expandEnvValue(value string, lenient bool) (string, error) {
	var expanded strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '$' || i+1 == len(value) {
			expanded.WriteByte(value[i])
			i++
			continue
		}
		switch value[i+1] {
		case '$':
			expanded.WriteByte('$')
			i += 2
		case '{':
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated environment reference")
			}
			end += i + 2
			name := value[i+2 : end]
			if !validEnvName(name) {
				return "", fmt.Errorf("invalid environment variable name %q", name)
			}
			replacement, ok := os.LookupEnv(name)
			if !ok {
				if lenient {
					expanded.WriteString(value[i : end+1])
					i = end + 1
					continue
				}
				return "", fmt.Errorf("environment variable %q is not set", name)
			}
			rememberSecret(replacement)
			expanded.WriteString(replacement)
			i = end + 1
		default:
			expanded.WriteByte(value[i])
			i++
		}
	}
	return expanded.String(), nil
}

func validEnvName(name string) bool {
	if name == "" || !isASCIILetter(name[0]) && name[0] != '_' {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isASCIILetter(name[i]) && (name[i] < '0' || name[i] > '9') && name[i] != '_' {
			return false
		}
	}
	return true
}

func isASCIILetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

// validateDataset checks that a dataset has a valid source configuration.
//
// A dataset must have either:
//   - A single "source" field, OR
//   - A "sources" list with at least one source
//
// It's an error to specify both, or to specify neither.
func validateDataset(ds *Dataset) error {
	hasSource := ds.Source.Type != ""
	hasSources := len(ds.Sources) > 0

	if !hasSource && !hasSources {
		return fmt.Errorf("dataset must have either 'source' or 'sources' specified")
	}

	if hasSource && hasSources {
		return fmt.Errorf("dataset cannot have both 'source' and 'sources' specified (use only one)")
	}

	return nil
}

// GetSources returns the list of sources for a dataset.
//
// This helper function normalizes the difference between single-source
// and multi-source configurations, always returning a slice of sources
// to simplify the logic in Check() and Fetch().
//
// For backward compatibility:
//   - If "source" is specified, returns a slice containing that single source
//   - If "sources" is specified, returns that slice
func (ds *Dataset) GetSources() []registry.Source {
	// If sources (plural) is specified, return it
	if len(ds.Sources) > 0 {
		return ds.Sources
	}
	// Otherwise, wrap the single source in a slice
	return []registry.Source{ds.Source}
}
