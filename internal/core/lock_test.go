package core

import (
	"os"
	"path/filepath"
	"runtime"
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

	t.Run("valid YAML with no items key initializes an empty map", func(t *testing.T) {
		// version-only content parses fine but leaves Items at its zero value (nil) - readLock
		// should defensively initialize it rather than leave callers to nil-check the map.
		path := filepath.Join(tmpDir, "no_items.lock.yaml")
		if err := os.WriteFile(path, []byte("version: 1\n"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		lk, err := readLock(path)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}
		if lk.Items == nil {
			t.Error("Items should be initialized to an empty map, not left nil")
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

	t.Run("write creates missing parent directories", func(t *testing.T) {
		// Regression test: writeLock used to assume the parent directory already existed,
		// unlike every handler's own file-write path (which os.MkdirAll's the target's parent
		// first) - a fresh --lock path pointing into a not-yet-created directory (a very
		// plausible first-run scenario) failed with a confusing "no such file or directory".
		lockPath := filepath.Join(tmpDir, "nested", "dir", "test.lock.yaml")
		lk := &Lock{Version: 1}

		if err := writeLock(lockPath, lk); err != nil {
			t.Fatalf("writeLock() error = %v, want nil (should create missing parent dirs)", err)
		}
		if _, err := os.Stat(lockPath); err != nil {
			t.Errorf("lock file was not created: %v", err)
		}
	})

	t.Run("write fails when a path component is a file, not a directory", func(t *testing.T) {
		// writeLock's own MkdirAll error branch (distinct from the "succeeds" case above):
		// mkdir can't create a directory through a path component that's already a regular file.
		blocker := filepath.Join(tmpDir, "blocker-file")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to create blocker file: %v", err)
		}
		lockPath := filepath.Join(blocker, "sub", "test.lock.yaml")

		if err := writeLock(lockPath, &Lock{Version: 1}); err == nil {
			t.Error("writeLock() expected error when parent path is blocked by a file, got nil")
		}
	})

	t.Run("write fails when the directory isn't writable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix permission bits don't apply the same way on Windows")
		}
		readOnlyDir := filepath.Join(tmpDir, "readonly")
		if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.Chmod(readOnlyDir, 0o555); err != nil {
			t.Fatalf("failed to chmod dir: %v", err)
		}
		defer func() {
			if err := os.Chmod(readOnlyDir, 0o755); err != nil {
				t.Logf("failed to restore dir permissions for cleanup: %v", err)
			}
		}()

		lockPath := filepath.Join(readOnlyDir, "test.lock.yaml")
		if err := writeLock(lockPath, &Lock{Version: 1}); err == nil {
			t.Error("writeLock() expected error when the directory isn't writable, got nil")
		}
	})

	t.Run("write and read inaccessible fields with special characters", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "inaccessible.lock.yaml")
		now := time.Now().UTC()

		// Create error message with special characters that might appear on Windows
		// Including backslashes, quotes, and newlines
		errorMsg := `failed to fetch: network error
path: C:\Users\test\file.txt
status: "connection timeout"`

		lk := &Lock{
			Version:     1,
			LastChecked: &now,
			Items: map[string]*LockItem{
				"dataset1": {
					LocalSHA256:       "hash123",
					RemoteFingerprint: "fp456",
					CheckedAt:         &now,
					InaccessibleAt:    &now,
					InaccessibleError: errorMsg,
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

		// Verify inaccessible fields are preserved
		item := readLk.Items["dataset1"]
		if item.InaccessibleAt == nil {
			t.Error("InaccessibleAt should not be nil")
		}
		if item.InaccessibleError != errorMsg {
			t.Errorf("InaccessibleError = %q, want %q", item.InaccessibleError, errorMsg)
		}
	})
}
