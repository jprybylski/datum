package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errorReader is an io.Reader that always fails, for exercising the "reading confirmation" error
// branch in Delete/Unlock - distinct from a declined ("n") answer or a closed stdin (io.EOF, which
// is treated as a decline).
type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) { return 0, errors.New("simulated read error") }

// deleteConfig writes a minimal one-dataset config using the "mock" handler (registered in
// engine_test.go) targeting targetFile, and returns the config path.
func deleteConfig(t *testing.T, dir, id, targetFile string) string {
	t.Helper()
	configPath := filepath.Join(dir, id+".data.yaml")
	mustWriteFile(t, configPath, []byte(`version: 1
datasets:
  - id: `+id+`
    source:
      type: mock
    target: `+targetFile+`
`))
	return configPath
}

func TestDelete_NoIDs(t *testing.T) {
	tmpDir := t.TempDir()
	var out bytes.Buffer
	code := Delete(filepath.Join(tmpDir, ".data.yaml"), filepath.Join(tmpDir, ".data.lock.yaml"), nil, true, nil, &out)
	if code != 2 {
		t.Errorf("Delete(no ids) = %d, want 2", code)
	}
}

func TestDelete_UnknownID(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "known", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"nope"}, true, nil, &out)
	if code != 2 {
		t.Errorf("Delete(unknown id) = %d, want 2", code)
	}
	if !strings.Contains(out.String(), "unknown dataset id: nope") {
		t.Errorf("output = %q, want it to mention the unknown id", out.String())
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Errorf("target file should be untouched by a failed delete: %v", err)
	}
}

func TestDelete_WithYesFlagRemovesFileAndMarksLockfile(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Delete() = %d, want 0; output: %s", code, out.String())
	}
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Errorf("target file should have been removed, stat err = %v", err)
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	item := lk.Items["ds1"]
	if item == nil || !item.Deleted {
		t.Fatalf("lock item = %+v, want Deleted=true", item)
	}
	if item.DeletedAt == nil {
		t.Error("DeletedAt should be set")
	}
	if item.RemoteFingerprint != "mock-fp" {
		t.Errorf("RemoteFingerprint = %q, want it preserved as mock-fp", item.RemoteFingerprint)
	}
}

func TestDelete_ConfirmationDeclined(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, false, strings.NewReader("n\n"), &out)
	if code != 0 {
		t.Fatalf("Delete() declined = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "aborted") {
		t.Errorf("output = %q, want it to mention aborting", out.String())
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Errorf("target file should survive a declined delete: %v", err)
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if item := lk.Items["ds1"]; item != nil && item.Deleted {
		t.Error("lock item should not be marked Deleted after declining")
	}
}

func TestDelete_ConfirmationAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, false, strings.NewReader("y\n"), &out)
	if code != 0 {
		t.Fatalf("Delete() accepted = %d, want 0; output: %s", code, out.String())
	}
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Errorf("target file should have been removed, stat err = %v", err)
	}
}

func TestDelete_ConfirmationReadError(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, false, errorReader{}, &out)
	if code != 1 {
		t.Errorf("Delete() with a failing confirmation reader = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "reading confirmation") {
		t.Errorf("output = %q, want it to mention the read error", out.String())
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Errorf("target file should survive a failed confirmation read: %v", err)
	}
}

func TestDelete_NeverFetchedTargetIsNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	// No lockfile and no target file at all - as if `datum fetch` was never run for this dataset.

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Delete() on a never-fetched dataset = %d, want 0; output: %s", code, out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if item := lk.Items["ds1"]; item == nil || !item.Deleted {
		t.Errorf("lock item = %+v, want Deleted=true even though there was nothing on disk", item)
	}
}

func TestDelete_FileRemovalError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(readOnlyDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, readOnlyDir, 0o755)

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 1 {
		t.Errorf("Delete() with an unremovable target = %d, want 1; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "[ERR ]") {
		t.Errorf("output = %q, want an [ERR ] line", out.String())
	}
}

func TestDelete_ConfirmationEOFTreatedAsDecline(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, false, strings.NewReader(""), &out)
	if code != 0 {
		t.Fatalf("Delete() EOF = %d, want 0 (treated as decline)", code)
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Errorf("target file should survive an EOF (no input) confirmation: %v", err)
	}
}

func TestDelete_AlreadyDeletedIsNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	if code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out); code != 0 {
		t.Fatalf("first Delete() = %d, want 0", code)
	}

	out.Reset()
	code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Errorf("second Delete() = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "already deleted") {
		t.Errorf("output = %q, want it to mention already deleted", out.String())
	}
}

func TestDelete_MultipleIDsOneUnknownAbortsAllBeforeDeleting(t *testing.T) {
	tmpDir := t.TempDir()
	target1 := filepath.Join(tmpDir, "target1.txt")
	target2 := filepath.Join(tmpDir, "target2.txt")
	configPath := filepath.Join(tmpDir, ".data.yaml")
	mustWriteFile(t, configPath, []byte(`version: 1
datasets:
  - id: ds1
    source:
      type: mock
    target: `+target1+`
  - id: ds2
    source:
      type: mock
    target: `+target2+`
`))
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, target1, []byte("data1"))
	mustWriteFile(t, target2, []byte("data2"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1", "nope"}, true, nil, &out)
	if code != 2 {
		t.Fatalf("Delete() = %d, want 2", code)
	}
	if _, err := os.Stat(target1); err != nil {
		t.Errorf("ds1's file should be untouched when a later id in the same call is unknown: %v", err)
	}
}

func TestUndelete_NoIDs(t *testing.T) {
	tmpDir := t.TempDir()
	var out bytes.Buffer
	code := Undelete(filepath.Join(tmpDir, ".data.lock.yaml"), nil, &out)
	if code != 2 {
		t.Errorf("Undelete(no ids) = %d, want 2", code)
	}
}

func TestUndelete_NotDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, lockPath, []byte("version: 1\nitems: {}\n"))

	var out bytes.Buffer
	code := Undelete(lockPath, []string{"nope"}, &out)
	if code != 1 {
		t.Errorf("Undelete(never deleted) = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "not marked as deleted") {
		t.Errorf("output = %q, want it to say not marked as deleted", out.String())
	}
}

func TestUndelete_ClearsDeletedFlag(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	if code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out); code != 0 {
		t.Fatalf("Delete() = %d, want 0", code)
	}

	out.Reset()
	code := Undelete(lockPath, []string{"ds1"}, &out)
	if code != 0 {
		t.Fatalf("Undelete() = %d, want 0; output: %s", code, out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	item := lk.Items["ds1"]
	if item == nil || item.Deleted {
		t.Fatalf("lock item = %+v, want Deleted=false", item)
	}
	if item.DeletedAt != nil {
		t.Error("DeletedAt should be cleared")
	}
}

func TestUndelete_ThenFetchRestoresData(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	if code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out); code != 0 {
		t.Fatalf("Delete() = %d, want 0", code)
	}
	if code := Undelete(lockPath, []string{"ds1"}, &out); code != 0 {
		t.Fatalf("Undelete() = %d, want 0", code)
	}

	if code := Fetch(context.Background(), configPath, lockPath, []string{"ds1"}, 1); code != 0 {
		t.Fatalf("Fetch() after undelete = %d, want 0", code)
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Errorf("target file should exist again after undelete + fetch: %v", err)
	}
}

func TestCheckFetch_SkipDeletedDataset(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
    deleted: true
    deleted_at: 2025-01-01T00:00:00Z
`))

	t.Run("check", func(t *testing.T) {
		if code := Check(context.Background(), configPath, lockPath, 1); code != 0 {
			t.Errorf("Check() on deleted dataset = %d, want 0 (skip, not fail)", code)
		}
		if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
			t.Errorf("Check() should not recreate a deleted target, stat err = %v", err)
		}
	})

	t.Run("fetch", func(t *testing.T) {
		if code := Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
			t.Errorf("Fetch() on deleted dataset = %d, want 0 (skip, not fail)", code)
		}
		if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
			t.Errorf("Fetch() should not recreate a deleted target, stat err = %v", err)
		}
	})

	// The lockfile's Deleted flag must survive being read/rewritten by Check and Fetch above.
	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if item := lk.Items["ds1"]; item == nil || !item.Deleted {
		t.Errorf("lock item = %+v, want Deleted still true after check/fetch", item)
	}
}

func TestDelete_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bad.yaml")
	mustWriteFile(t, configPath, []byte("bad: yaml: syntax:"))

	var out bytes.Buffer
	code := Delete(configPath, filepath.Join(tmpDir, ".data.lock.yaml"), []string{"ds1"}, true, nil, &out)
	if code != 2 {
		t.Errorf("Delete() with invalid config = %d, want 2", code)
	}
	if !strings.Contains(out.String(), "config error") {
		t.Errorf("output = %q, want it to mention a config error", out.String())
	}
}

func TestDelete_LockReadError(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, "bad.lock.yaml")
	mustWriteFile(t, lockPath, []byte("bad: yaml: syntax:"))

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 2 {
		t.Errorf("Delete() with invalid lock = %d, want 2", code)
	}
	if !strings.Contains(out.String(), "lock error") {
		t.Errorf("output = %q, want it to mention a lock error", out.String())
	}
}

func TestDelete_LockWriteError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	mustWriteFile(t, targetFile, []byte("data"))

	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, readOnlyDir, 0o755)
	lockPath := filepath.Join(readOnlyDir, "lock.yaml")

	var out bytes.Buffer
	code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 1 {
		t.Errorf("Delete() with unwritable lock dir = %d, want 1", code)
	}
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Errorf("target file should still be removed even though the lockfile couldn't be written: stat err = %v", err)
	}
}

func TestUndelete_LockReadError(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "bad.lock.yaml")
	mustWriteFile(t, lockPath, []byte("bad: yaml: syntax:"))

	var out bytes.Buffer
	code := Undelete(lockPath, []string{"ds1"}, &out)
	if code != 2 {
		t.Errorf("Undelete() with invalid lock = %d, want 2", code)
	}
}

func TestUndelete_LockWriteError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(readOnlyDir, "lock.yaml")
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    deleted: true
`))
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, readOnlyDir, 0o755)

	var out bytes.Buffer
	code := Undelete(lockPath, []string{"ds1"}, &out)
	if code != 1 {
		t.Errorf("Undelete() with unwritable lock dir = %d, want 1", code)
	}
}

func TestCheck_SkipDeletedDataset_JSONStatus(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
    deleted: true
`))

	var out string
	var code int
	withJSONOutput(t, func() {
		out = captureStdout(t, func() {
			code = Check(context.Background(), configPath, lockPath, 1)
		})
	})
	if code != 0 {
		t.Fatalf("Check() = %d, want 0", code)
	}
	if !strings.Contains(out, `"status": "deleted"`) {
		t.Errorf("output = %s, want status=deleted", out)
	}
}
