package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnlock_NoIDs(t *testing.T) {
	tmpDir := t.TempDir()
	var out bytes.Buffer
	code := Unlock(filepath.Join(tmpDir, ".data.yaml"), filepath.Join(tmpDir, ".data.lock.yaml"), nil, true, nil, &out)
	if code != 2 {
		t.Errorf("Unlock(no ids) = %d, want 2", code)
	}
}

func TestUnlock_LockReadError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, "bad.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte("bad: yaml: syntax:"))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 2 {
		t.Errorf("Unlock() with invalid lock = %d, want 2", code)
	}
	if !strings.Contains(out.String(), "lock error") {
		t.Errorf("output = %q, want it to mention a lock error", out.String())
	}
}

func TestUnlock_ConfirmationReadError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, false, errorReader{}, &out)
	if code != 1 {
		t.Errorf("Unlock() with a failing confirmation reader = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "reading confirmation") {
		t.Errorf("output = %q, want it to mention the read error", out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; !ok {
		t.Error("ds1 should survive a failed confirmation read")
	}
}

func TestUnlock_MultipleIDsConfirmationPluralizesCorrectly(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp-1
  ds2:
    remote_fingerprint: mock-fp-2
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1", "ds2"}, false, strings.NewReader("y\n"), &out)
	if code != 0 {
		t.Fatalf("Unlock() = %d, want 0; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "Unlock 2 entries?") {
		t.Errorf("output = %q, want the plural prompt for 2 entries", out.String())
	}
}

func TestUnlock_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte("version: 1\nitems: {}\n"))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"nope"}, true, nil, &out)
	if code != 1 {
		t.Errorf("Unlock(not found) = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "no lockfile entry") {
		t.Errorf("output = %q, want it to mention no lockfile entry", out.String())
	}
}

func TestUnlock_OrphanedEntry(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	// ds1 only exists in the lockfile, not the config - as if the user removed it by hand.
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Unlock(orphaned) = %d, want 0; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "orphaned - not in .data.yaml") {
		t.Errorf("output = %q, want it to note the entry is orphaned", out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; ok {
		t.Error("ds1 should have been removed from the lockfile")
	}
}

func TestUnlock_ActiveEntry(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Unlock(active) = %d, want 0; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "resets its pin") {
		t.Errorf("output = %q, want it to warn about resetting the pin for a still-tracked dataset", out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; ok {
		t.Error("ds1 should have been removed from the lockfile even though it's still in the config")
	}
}

func TestUnlock_DeletedEntry(t *testing.T) {
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
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Unlock(deleted) = %d, want 0; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "deleted, still in .data.yaml") {
		t.Errorf("output = %q, want it to note the entry is deleted and still tracked", out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; ok {
		t.Error("ds1 should have been fully removed from the lockfile")
	}
}

func TestUnlock_DeletedAndOrphanedEntry(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	configPath := deleteConfig(t, tmpDir, "ds1", targetFile)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, targetFile, []byte("data"))

	var out bytes.Buffer
	if code := Delete(configPath, lockPath, []string{"ds1"}, true, nil, &out); code != 0 {
		t.Fatalf("Delete() = %d, want 0", code)
	}

	// Now remove ds1 from the config entirely, on top of it already being deleted.
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))

	out.Reset()
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Unlock(deleted+orphaned) = %d, want 0; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "deleted, orphaned - not in .data.yaml") {
		t.Errorf("output = %q, want it to note the entry is both deleted and orphaned", out.String())
	}
}

func TestUnlock_ConfirmationDeclined(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, false, strings.NewReader("n\n"), &out)
	if code != 0 {
		t.Fatalf("Unlock() declined = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "aborted") {
		t.Errorf("output = %q, want it to mention aborting", out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; !ok {
		t.Error("ds1 should still be in the lockfile after declining")
	}
}

func TestUnlock_ConfirmationAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, false, strings.NewReader("y\n"), &out)
	if code != 0 {
		t.Fatalf("Unlock() accepted = %d, want 0; output: %s", code, out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; ok {
		t.Error("ds1 should have been removed after accepting")
	}
}

func TestUnlock_ConfigReadFailureStillWorks(t *testing.T) {
	tmpDir := t.TempDir()
	// A missing/invalid config must not block unlocking - it's fundamentally a lockfile-only
	// operation, and the config is only consulted best-effort to annotate the confirmation text.
	configPath := filepath.Join(tmpDir, "missing.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Unlock() with unreadable config = %d, want 0; output: %s", code, out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; ok {
		t.Error("ds1 should have been removed despite the config being unreadable")
	}
}

func TestUnlock_MultipleIDsMixedFoundAndNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1", "nope"}, true, nil, &out)
	if code != 1 {
		t.Errorf("Unlock(mixed) = %d, want 1 (one id not found)", code)
	}
	if !strings.Contains(out.String(), "no lockfile entry") {
		t.Errorf("output = %q, want it to mention the missing id", out.String())
	}
	if !strings.Contains(out.String(), "unlocked") {
		t.Errorf("output = %q, want the found id to still be unlocked", out.String())
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["ds1"]; ok {
		t.Error("ds1 should still have been unlocked despite the other id being missing")
	}
}

func TestUnlock_LockWriteError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))

	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(readOnlyDir, "lock.yaml")
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  ds1:
    remote_fingerprint: mock-fp
`))
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, readOnlyDir, 0o755)

	var out bytes.Buffer
	code := Unlock(configPath, lockPath, []string{"ds1"}, true, nil, &out)
	if code != 1 {
		t.Errorf("Unlock() with unwritable lock dir = %d, want 1", code)
	}
}
