package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadLock(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid lock file", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "valid.lock.yaml")
		lockContent := `version: 1
last_checked: 2025-10-24T12:00:00Z
items:
  test_dataset:
    local_sha256: abc123
    remote_fingerprint: def456
    checked_at: 2025-10-24T12:00:00Z
`
		if err := os.WriteFile(lockPath, []byte(lockContent), 0o644); err != nil {
			t.Fatalf("failed to create test lock file: %v", err)
		}

		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		if lk.Version != 1 {
			t.Errorf("Version = %v, want 1", lk.Version)
		}
		if lk.Items == nil {
			t.Fatal("Items is nil")
		}
		if len(lk.Items) != 1 {
			t.Fatalf("len(Items) = %v, want 1", len(lk.Items))
		}
		item, ok := lk.Items["test_dataset"]
		if !ok {
			t.Fatal("test_dataset not found in Items")
		}
		if item.LocalSHA256 != "abc123" {
			t.Errorf("LocalSHA256 = %v, want abc123", item.LocalSHA256)
		}
		if item.RemoteFingerprint != "def456" {
			t.Errorf("RemoteFingerprint = %v, want def456", item.RemoteFingerprint)
		}
	})

	t.Run("non-existent file returns empty lock", func(t *testing.T) {
		lk, err := readLock(filepath.Join(tmpDir, "nonexistent.lock.yaml"))
		// readLock should return an empty lock, not an error
		if err != nil {
			t.Errorf("readLock() unexpected error = %v", err)
		}
		// Lock items may be nil for an empty lock - this is acceptable
		if len(lk.Items) > 0 {
			t.Errorf("readLock() expected empty items, got %d items", len(lk.Items))
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		invalidPath := filepath.Join(tmpDir, "invalid.lock.yaml")
		invalidContent := "this is not: valid: yaml: content:"
		if err := os.WriteFile(invalidPath, []byte(invalidContent), 0o644); err != nil {
			t.Fatalf("failed to create invalid lock file: %v", err)
		}

		_, err := readLock(invalidPath)
		if err == nil {
			t.Error("readLock() expected error for invalid YAML, got nil")
		}
	})
}

func TestWriteLock(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("write and read back", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "test.lock.yaml")
		now := time.Now().UTC()

		// Create a lock structure
		lk := &Lock{
			Version:     1,
			LastChecked: &now,
			Items: map[string]*LockItem{
				"dataset1": {
					LocalSHA256:       "hash123",
					RemoteFingerprint: "fp456",
					CheckedAt:         &now,
				},
			},
		}

		// Write it
		if err := writeLock(lockPath, lk); err != nil {
			t.Fatalf("writeLock() error = %v", err)
		}

		// Read it back
		readLk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		// Verify
		if readLk.Version != 1 {
			t.Errorf("Version = %v, want 1", readLk.Version)
		}
		if len(readLk.Items) != 1 {
			t.Fatalf("len(Items) = %v, want 1", len(readLk.Items))
		}
		item := readLk.Items["dataset1"]
		if item.LocalSHA256 != "hash123" {
			t.Errorf("LocalSHA256 = %v, want hash123", item.LocalSHA256)
		}
		if item.RemoteFingerprint != "fp456" {
			t.Errorf("RemoteFingerprint = %v, want fp456", item.RemoteFingerprint)
		}
	})

	t.Run("write to non-existent directory fails", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "nonexistent", "dir", "test.lock.yaml")
		lk := &Lock{Version: 1}

		err := writeLock(lockPath, lk)
		if err == nil {
			t.Error("writeLock() expected error for non-existent directory, got nil")
		}
	})
}
