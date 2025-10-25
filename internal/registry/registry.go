// Package registry provides a plugin-based handler registration system for datum.
//
// This package enables datum's extensible architecture by allowing different data source
// handlers (HTTP, file, git, command) to register themselves at program startup.
// Handlers are registered via init() functions and looked up by their type name.
//
// Key concepts for Go beginners:
//   - The registry uses a global map (package-level variable) to store handlers
//   - Handlers self-register using init() functions which run before main()
//   - The Fetcher interface provides polymorphism - any type implementing these methods can be a handler
package registry

import "context"

// Source represents the configuration for a data source.
// It contains fields used by various handler types. Not all fields are used by all handlers.
//
// YAML tags control how this struct is serialized/deserialized from configuration files.
// The `omitempty` tag means the field will be omitted from YAML if it's empty.
type Source struct {
	Type string `yaml:"type"`           // Handler type: "http", "file", "git", or "command"
	URL  string `yaml:"url,omitempty"`  // URL for http and git handlers
	Path string `yaml:"path,omitempty"` // File path for file and git handlers
	Ref  string `yaml:"ref,omitempty"`  // Git ref (branch/tag) for git handler

	// Command handler specific fields
	FingerprintCmd string `yaml:"fingerprint_cmd,omitempty"` // Command to compute fingerprint
	FetchCmd       string `yaml:"fetch_cmd,omitempty"`       // Command to fetch data
}

// Fetcher is the interface that all data source handlers must implement.
//
// This is an example of Go's interface-based polymorphism. Any type that has these
// three methods automatically satisfies this interface without explicit declaration.
//
// Context is passed to enable cancellation and timeouts.
type Fetcher interface {
	// Name returns the handler's type identifier (e.g., "http", "file", "git", "command").
	// This name is used to look up the handler when processing datasets.
	Name() string

	// Fingerprint computes a stable identifier for the data source without downloading it.
	// Different handlers use different strategies (ETag, file hash, git blob SHA, etc.).
	// Returns a fingerprint string or an error if computation fails.
	Fingerprint(ctx context.Context, src Source) (string, error)

	// Fetch downloads or copies the data from the source to the destination file.
	// The dest parameter is the local file path where data should be written.
	// Returns an error if the fetch operation fails.
	Fetch(ctx context.Context, src Source, dest string) error
}

// fetchers is the global registry of all available handlers.
// This is a package-level variable that persists for the lifetime of the program.
// It's populated by handler init() functions at startup.
var fetchers = map[string]Fetcher{}

// Register adds a handler to the global registry.
// This function is typically called from handler packages' init() functions.
//
// Example usage in a handler package:
//
//	func init() {
//	    registry.Register(New())
//	}
func Register(f Fetcher) { fetchers[f.Name()] = f }

// Get retrieves a handler by its type name.
// Returns the handler and true if found, or nil and false if not found.
//
// The boolean return value follows Go's "comma ok" idiom for safe map lookups.
func Get(kind string) (Fetcher, bool) {
	f, ok := fetchers[kind]
	return f, ok
}
