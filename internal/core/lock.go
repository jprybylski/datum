package core

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Lock represents the lockfile structure that tracks dataset fingerprints.
//
// The lockfile serves as a record of the last known state of all datasets.
// It's similar to package-lock.json for npm or go.sum for Go modules, but for data files.
//
// This struct is serialized to/from YAML (.data.lock.yaml file).
type Lock struct {
	Version     int                  `yaml:"version"`                // Lockfile format version (currently 1)
	LastChecked *time.Time           `yaml:"last_checked,omitempty"` // Timestamp of last check operation
	Items       map[string]*LockItem `yaml:"items"`                  // Map of dataset ID to lock item
}

// LockItem stores the verification state for a single dataset.
//
// Each dataset in the configuration has a corresponding LockItem that records:
//   - The local file's hash (to detect local modifications)
//   - The remote source's fingerprint (to detect upstream changes)
//   - When it was last verified
type LockItem struct {
	LocalSHA256       string     `yaml:"local_sha256,omitempty"`       // SHA256 hash of the local file
	RemoteFingerprint string     `yaml:"remote_fingerprint,omitempty"` // Remote fingerprint (ETag, git SHA, etc.)
	CheckedAt         *time.Time `yaml:"checked_at,omitempty"`         // Last verification timestamp
}

// readLock loads the lockfile from disk.
//
// If the lockfile doesn't exist, this returns an empty Lock instead of an error.
// This allows the first run to create a new lockfile without special handling.
//
// Parameters:
//   - path: Path to the lockfile (typically .data.lock.yaml)
//
// Returns:
//   - A Lock struct populated from the file, or an empty Lock if the file doesn't exist
//   - An error if the file exists but can't be parsed as valid YAML
//
// Go learning note: Using a pointer return type (*Lock) allows returning nil and
// enables modification of the Lock without copying the entire struct.
func readLock(path string) (*Lock, error) {
	// Try to read the lockfile
	b, err := os.ReadFile(path)
	if err != nil {
		// If the file doesn't exist, return an empty lock (not an error)
		// This is intentional - the first run will create the lockfile
		return &Lock{Version: 1, Items: map[string]*LockItem{}}, nil
	}

	// Parse the YAML into a Lock struct
	var l Lock
	if err := yaml.Unmarshal(b, &l); err != nil {
		return nil, err
	}

	// Ensure Items map is initialized (defensive programming)
	// If YAML is empty or malformed, Items might be nil
	if l.Items == nil {
		l.Items = map[string]*LockItem{}
	}

	return &l, nil
}

// writeLock saves the lockfile to disk atomically.
//
// To prevent corruption from crashes or interrupts, this function uses atomic writes:
//  1. Write to a temporary file (.data.lock.yaml.tmp)
//  2. Rename the temporary file to the final name
//
// On Unix systems, rename is atomic, so the lockfile is never in a partially-written state.
//
// Parameters:
//   - path: Path where the lockfile should be written
//   - l: The Lock struct to serialize
//
// Returns:
//   - An error if marshaling, writing, or renaming fails
//
// Go learning note: The 0o644 is an octal file permission (readable by all, writable by owner).
// The 'o' prefix indicates octal notation (base 8), a Go 1.13+ feature.
func writeLock(path string, l *Lock) error {
	// Marshal the Lock struct to YAML bytes
	b, err := yaml.Marshal(l)
	if err != nil {
		return err
	}

	// Write to a temporary file first (atomic write pattern)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}

	// Atomically rename temporary file to final destination
	// If this succeeds, the file is guaranteed to be complete
	return os.Rename(tmp, path)
}
