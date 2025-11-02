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
	Policy  string            `yaml:"policy"`            // Policy override (empty uses default)
	Source  registry.Source   `yaml:"source,omitempty"`  // Single data source (backward compatible)
	Sources []registry.Source `yaml:"sources,omitempty"` // Multiple data sources with fallback
}

// readConfig loads and parses the configuration file from disk.
//
// The function reads the YAML file, unmarshals it into a Config struct,
// and applies default values for any unspecified settings.
//
// Parameters:
//   - path: Path to the configuration file (typically .data.yaml)
//
// Returns:
//   - A pointer to the parsed Config struct
//   - An error if the file cannot be read or parsed
//
// Go learning note: This function applies "sensible defaults" - if a field
// is not specified in the YAML, it gets a reasonable default value.
func readConfig(path string) (*Config, error) {
	// Read the entire file into a byte slice
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse the YAML into a Config struct
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	// Apply default values if not specified in the configuration
	// This ensures the config always has valid values even if the user
	// doesn't explicitly set them
	if c.Defaults.Policy == "" {
		c.Defaults.Policy = "fail" // Default to strict mode
	}
	if c.Defaults.Algo == "" {
		c.Defaults.Algo = "sha256" // Default to SHA256 hashing
	}

	// Validate dataset configurations
	for i, ds := range c.Datasets {
		if err := validateDataset(&ds); err != nil {
			return nil, fmt.Errorf("dataset %d (%s): %w", i, ds.ID, err)
		}
	}

	return &c, nil
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
