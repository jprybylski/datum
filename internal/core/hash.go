package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// HashFile computes the SHA256 hash of a file's contents.
//
// This function is used to verify that local files haven't been modified.
// It returns the hash as a lowercase hexadecimal string.
//
// The implementation uses io.Copy for efficient hashing of large files without
// loading the entire file into memory at once.
//
// Parameters:
//   - path: Absolute or relative path to the file to hash
//
// Returns:
//   - A 64-character hexadecimal string (256 bits / 4 bits per hex char = 64 chars)
//   - An error if the file cannot be opened or read
//
// Go learning note: The defer statement ensures f.Close() is called when the function
// returns, even if an error occurs. This is Go's idiom for resource cleanup.
func HashFile(path string) (string, error) {
	// Open the file for reading
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() // Ensure file is closed when function exits

	// Create a new SHA256 hasher
	// The hasher implements io.Writer, so we can copy data directly to it
	h := sha256.New()

	// Copy the file contents to the hasher
	// This is efficient for large files as it streams data in chunks
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	// Sum(nil) returns the hash as a byte slice
	// EncodeToString converts it to a readable hexadecimal string
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileExists checks whether a file or directory exists at the given path.
//
// This is a simple utility function used throughout the codebase to verify
// that target files exist before attempting to hash them.
//
// Parameters:
//   - path: File or directory path to check
//
// Returns:
//   - true if the path exists (file or directory)
//   - false if the path doesn't exist or if there's an error accessing it
//
// Go learning note: os.Stat returns file metadata or an error if the file doesn't exist.
// We only care about existence, so we discard the metadata with underscore (_).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// firstNonEmpty returns the first non-empty string from two options.
//
// This is a simple utility function used to provide default values.
// Common use case: dataset.Policy (if set) or config.Defaults.Policy (fallback).
//
// Parameters:
//   - a: First string to check (higher priority)
//   - b: Second string to use if a is empty (fallback)
//
// Returns:
//   - a if it's not empty, otherwise b
//
// Go learning note: len() on a string returns the number of bytes, not characters.
// For ASCII strings this is the same, but for Unicode it may differ.
func firstNonEmpty(a, b string) string {
	if len(a) > 0 {
		return a
	}
	return b
}
