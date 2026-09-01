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

import (
	"context"
	"sort"
)

// Source represents the configuration for a data source.
// It contains fields used by various handler types. Not all fields are used by all handlers.
//
// YAML tags control how this struct is serialized/deserialized from configuration files.
// The `omitempty` tag means the field will be omitted from YAML if it's empty.
type Source struct {
	Type    string            `yaml:"type"`              // Handler type: "http", "file", "git", or "command"
	URL     string            `yaml:"url,omitempty"`     // URL for http and git handlers
	Path    string            `yaml:"path,omitempty"`    // File path for file and git handlers
	Ref     string            `yaml:"ref,omitempty"`     // Git ref (branch/tag) for git handler
	Headers map[string]string `yaml:"headers,omitempty"` // HTTP request headers
	Body    string            `yaml:"body,omitempty"`    // HTTP request body; a non-empty body implies POST

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

// FingerprintingFetcher is an optional interface for handlers that can derive the remote
// fingerprint from the same response or operation used to fetch the target. It avoids a second
// request and ensures the recorded fingerprint describes the bytes that were actually written.
type FingerprintingFetcher interface {
	Fetcher
	FetchWithFingerprint(ctx context.Context, src Source, dest string) (fingerprint string, err error)
}

// DirManifestFetcher is an optional interface for handlers whose Fetch may populate a directory
// tree rather than write a single file. When a handler implements it, the engine calls FetchDir
// instead of Fetch, threading through the manifest of relative paths this same dataset wrote
// under dest on its previous run (so the handler knows what to remove if a file disappears from
// the source) and getting the new manifest back to persist. The engine stores that manifest on
// the dataset's LockItem, not a sidecar file the handler would otherwise have to write to disk
// itself - manifest state belongs in the lockfile alongside the rest of a dataset's tracked
// state, not scattered next to fetched data.
//
// claimed lists relative paths that, as of the lockfile, belong to a *different* dataset that
// targets the same dest directory - multiple datasets are allowed to share a target as long as
// they don't write the same relative path. A handler should fail the fetch rather than write any
// path in claimed, since that would silently overwrite (or be silently overwritten by) that other
// dataset's file.
//
// For a source that turns out to be a single file rather than a directory, FetchDir should behave
// like Fetch and return a nil manifest.
type DirManifestFetcher interface {
	Fetcher
	FetchDir(ctx context.Context, src Source, dest string, prevManifest []string, claimed map[string]bool) (manifest []string, err error)
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

// Names returns the source types available in this build, sorted for stable CLI output. Optional
// handlers such as git only appear when their package was included by the build tags.
func Names() []string {
	names := make([]string, 0, len(fetchers))
	for name := range fetchers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
