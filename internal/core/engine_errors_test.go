package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jprybylski/datum/internal/registry"
)

// mockHandlerUnreadableTarget fetches successfully but leaves the target file unreadable
// afterward, so the post-fetch HashPath call in checkOneDataset/fetchOneDataset fails even
// though the fetch itself succeeded - distinct from a fetch failure.
type mockHandlerUnreadableTarget struct{ name string }

func (m *mockHandlerUnreadableTarget) Name() string { return m.name }

func (m *mockHandlerUnreadableTarget) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	return m.name + "-fp", nil
}

func (m *mockHandlerUnreadableTarget) Fetch(ctx context.Context, src registry.Source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, []byte("data"), 0o644); err != nil {
		return err
	}
	return os.Chmod(dest, 0o000)
}

// mockHandlerFetchOKFingerprintFails always fetches successfully but always fails to
// fingerprint - exercises fetchAttempt's "fetch succeeded, but the required post-fetch
// re-fingerprint didn't" branch, distinct from a plain fetch failure.
type mockHandlerFetchOKFingerprintFails struct{ name string }

func (m *mockHandlerFetchOKFingerprintFails) Name() string { return m.name }

func (m *mockHandlerFetchOKFingerprintFails) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	return "", errors.New("fingerprint always fails")
}

func (m *mockHandlerFetchOKFingerprintFails) Fetch(ctx context.Context, src registry.Source, dest string) error {
	return os.WriteFile(dest, []byte("data"), 0o644)
}

func init() {
	registry.Register(&mockHandlerUnreadableTarget{name: "mockunreadable"})
	registry.Register(&mockHandlerFetchOKFingerprintFails{name: "mockfpfail"})
}

func skipUnlessCanDenyRead(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
}

// chmodOrLog restores permissions during test cleanup; failures are logged, not fatal, since
// they'd only affect how cleanly t.TempDir() can remove the directory afterward.
func chmodOrLog(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Logf("cleanup: failed to chmod %s: %v", path, err)
	}
}

// --- sourceAttempt direct tests ---

func TestSourceAttempt_UnknownSourceType(t *testing.T) {
	attempt := func(f registry.Fetcher, source registry.Source) (string, string, error) {
		t.Fatal("attempt should never be called for an unregistered source type")
		return "", "", nil
	}

	t.Run("single source", func(t *testing.T) {
		var w strings.Builder
		_, err := sourceAttempt(&w, nil, "ds1", []registry.Source{{Type: "nonexistent-handler"}}, attempt)
		if err == nil {
			t.Fatal("expected error for unknown source type, got nil")
		}
		if !strings.Contains(err.Error(), "unknown source.type") {
			t.Errorf("error = %v, want it to mention unknown source.type", err)
		}
		// Single source: no [WARN] progress line should be printed (only the caller's final [ERR ]).
		if w.String() != "" {
			t.Errorf("expected no warn output for single unknown source, got %q", w.String())
		}
	})

	t.Run("multiple sources prints a warn line per failure", func(t *testing.T) {
		var w strings.Builder
		sources := []registry.Source{{Type: "nonexistent-a"}, {Type: "nonexistent-b"}}
		_, err := sourceAttempt(&w, nil, "ds1", sources, attempt)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		out := w.String()
		if !strings.Contains(out, "1/2") || !strings.Contains(out, "2/2") {
			t.Errorf("expected [WARN] lines for both sources, got %q", out)
		}
	})
}

func TestSourceAttempt_EmptyWarnLabel(t *testing.T) {
	// fingerprintAttempt/fetchAttempt always supply a non-empty warnLabel on error, so this
	// exercises sourceAttempt's own generic contract directly (it's meant to be reusable beyond
	// just those two builders) rather than only through them.
	var w strings.Builder
	sources := []registry.Source{{Type: "mock"}, {Type: "mockfail"}}
	attempt := func(f registry.Fetcher, source registry.Source) (string, string, error) {
		return "", "", errors.New("boom")
	}
	_, err := sourceAttempt(&w, nil, "ds1", sources, attempt)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	out := w.String()
	// No label prefix (e.g. "fetch: boom") should appear between "source N/2:" and the error text.
	if !strings.Contains(out, "source 1/2: boom (trying next source)") {
		t.Errorf("expected an unlabeled warn line, got %q", out)
	}
}

func TestFetchAttempt_FetchSucceedsFingerprintFails(t *testing.T) {
	f, ok := registry.Get("mockfpfail")
	if !ok {
		t.Fatal("mockfpfail handler not registered")
	}
	dest := filepath.Join(t.TempDir(), "target.txt")
	attempt := fetchAttempt(context.Background(), dest)

	value, warnLabel, err := attempt(f, registry.Source{Type: "mockfpfail"})
	if err == nil {
		t.Fatal("expected an error when the post-fetch re-fingerprint fails, got nil")
	}
	if warnLabel != "fingerprint after fetch" {
		t.Errorf("warnLabel = %q, want %q", warnLabel, "fingerprint after fetch")
	}
	if value != "" {
		t.Errorf("value = %q, want empty string on failure", value)
	}
	// The fetch itself did succeed, so the file should exist even though the overall attempt
	// is reported as failed (matching fetchAttempt's "only counts as succeeded if both steps
	// succeed" contract).
	if _, statErr := os.Stat(dest); statErr != nil {
		t.Errorf("expected the fetched file to still exist: %v", statErr)
	}
}

// --- checkOneDataset / Check branch tests ---

func TestCheckOneDataset_LocalHashError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, target, 0o644)

	ds := Dataset{ID: "ds1", Target: target, Source: registry.Source{Type: "mock"}}
	store := &lockStore{lk: &Lock{Items: map[string]*LockItem{}}}
	var w strings.Builder

	code := checkOneDataset(context.Background(), &w, nil, ds, "log", store, time.Now().UTC())
	if code != 0 {
		t.Errorf("checkOneDataset() = %d, want 0 (log policy doesn't fail on local hash error)", code)
	}
	if !strings.Contains(w.String(), "local hash") {
		t.Errorf("expected output to mention the local hash error, got %q", w.String())
	}
}

func TestCheckOneDataset_UpdatePolicy_AllSourcesFailToFetch(t *testing.T) {
	ds := Dataset{
		ID:     "ds1",
		Target: filepath.Join(t.TempDir(), "target.txt"),
		Sources: []registry.Source{
			{Type: "mockfail"},
			{Type: "mockfail"},
		},
	}
	store := &lockStore{lk: &Lock{Items: map[string]*LockItem{}}}
	var w strings.Builder
	now := time.Now().UTC()

	code := checkOneDataset(context.Background(), &w, nil, ds, "update", store, now)
	if code != 1 {
		t.Errorf("checkOneDataset() = %d, want 1", code)
	}
	item := store.get("ds1")
	if item == nil || item.InaccessibleAt == nil {
		t.Fatal("expected InaccessibleAt to be set after all sources failed to fetch")
	}
	if !strings.Contains(w.String(), "all 2 sources failed to fetch") {
		t.Errorf("expected multi-source fetch failure message, got %q", w.String())
	}
}

func TestCheckOneDataset_UpdatePolicy_LocalHashErrorAfterFetch(t *testing.T) {
	skipUnlessCanDenyRead(t)
	target := filepath.Join(t.TempDir(), "target.txt")
	ds := Dataset{ID: "ds1", Target: target, Source: registry.Source{Type: "mockunreadable"}}
	store := &lockStore{lk: &Lock{Items: map[string]*LockItem{}}}
	var w strings.Builder

	code := checkOneDataset(context.Background(), &w, nil, ds, "update", store, time.Now().UTC())
	if code != 0 {
		t.Errorf("checkOneDataset() = %d, want 0 (fetch succeeded even though hashing failed after)", code)
	}
	if !strings.Contains(w.String(), "local hash after fetch") {
		t.Errorf("expected a warning about hashing after fetch, got %q", w.String())
	}
	chmodOrLog(t, target, 0o644) // let TempDir clean up
}

func TestCheckOneDataset_LogPolicy_NotStale(t *testing.T) {
	ds := Dataset{ID: "ds1", Target: filepath.Join(t.TempDir(), "target.txt"), Source: registry.Source{Type: "mock"}}
	store := &lockStore{lk: &Lock{Items: map[string]*LockItem{
		"ds1": {RemoteFingerprint: "mock-fp"},
	}}}
	var w strings.Builder

	code := checkOneDataset(context.Background(), &w, nil, ds, "log", store, time.Now().UTC())
	if code != 0 {
		t.Errorf("checkOneDataset() = %d, want 0", code)
	}
	if !strings.Contains(w.String(), "[OK  ]") {
		t.Errorf("expected up-to-date [OK] line, got %q", w.String())
	}
}

func TestCheckOneDataset_FailPolicy_NotStale(t *testing.T) {
	ds := Dataset{ID: "ds1", Target: filepath.Join(t.TempDir(), "target.txt"), Source: registry.Source{Type: "mock"}}
	store := &lockStore{lk: &Lock{Items: map[string]*LockItem{
		"ds1": {RemoteFingerprint: "mock-fp"},
	}}}
	var w strings.Builder

	code := checkOneDataset(context.Background(), &w, nil, ds, "fail", store, time.Now().UTC())
	if code != 0 {
		t.Errorf("checkOneDataset() = %d, want 0", code)
	}
	if !strings.Contains(w.String(), "[OK  ]") {
		t.Errorf("expected up-to-date [OK] line, got %q", w.String())
	}
}

func TestCheckOneDataset_UnknownPolicy(t *testing.T) {
	t.Run("stale", func(t *testing.T) {
		ds := Dataset{ID: "ds1", Target: filepath.Join(t.TempDir(), "target.txt"), Source: registry.Source{Type: "mock"}}
		store := &lockStore{lk: &Lock{Items: map[string]*LockItem{}}}
		var w strings.Builder

		code := checkOneDataset(context.Background(), &w, nil, ds, "bogus-policy", store, time.Now().UTC())
		if code != 1 {
			t.Errorf("checkOneDataset() = %d, want 1 (unknown policy treated as fail when stale)", code)
		}
		if !strings.Contains(w.String(), `unknown policy="bogus-policy"`) {
			t.Errorf("expected unknown-policy warning, got %q", w.String())
		}
	})

	t.Run("not stale", func(t *testing.T) {
		ds := Dataset{ID: "ds1", Target: filepath.Join(t.TempDir(), "target.txt"), Source: registry.Source{Type: "mock"}}
		store := &lockStore{lk: &Lock{Items: map[string]*LockItem{
			"ds1": {RemoteFingerprint: "mock-fp"},
		}}}
		var w strings.Builder

		code := checkOneDataset(context.Background(), &w, nil, ds, "bogus-policy", store, time.Now().UTC())
		if code != 0 {
			t.Errorf("checkOneDataset() = %d, want 0 (unknown policy, but not stale)", code)
		}
	})
}

func TestCheck_WriteLockError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	targetFile := filepath.Join(tmpDir, "target.txt")
	// policy: log never contributes to the exit code regardless of staleness, so exit is still 0
	// going into the final writeLock call - otherwise the "first failure" (exit == 0 -> 1) branch
	// specifically wouldn't be exercised, only the "already nonzero, stays nonzero" one.
	configContent := "version: 1\ndatasets:\n  - id: ds1\n    source:\n      type: mock\n    target: " + targetFile + "\n    policy: log\n"
	mustWriteFile(t, configPath, []byte(configContent))

	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, readOnlyDir, 0o755)
	lockPath := filepath.Join(readOnlyDir, "lock.yaml")

	code := Check(context.Background(), configPath, lockPath, 1)
	if code != 1 {
		t.Errorf("Check() = %d, want 1 when the lockfile can't be written", code)
	}
}

// --- Fetch branch tests ---

func TestFetch_ExplicitIDsFilterDatasets(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `version: 1
datasets:
  - id: wanted
    source:
      type: mock
    target: ` + filepath.Join(tmpDir, "wanted.txt") + `
  - id: skipped
    source:
      type: mock
    target: ` + filepath.Join(tmpDir, "skipped.txt") + `
`
	mustWriteFile(t, configPath, []byte(configContent))
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	code := Fetch(context.Background(), configPath, lockPath, []string{"wanted"}, 1)
	if code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "wanted.txt")); err != nil {
		t.Errorf("wanted.txt should have been fetched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "skipped.txt")); !os.IsNotExist(err) {
		t.Errorf("skipped.txt should not have been fetched, stat err = %v", err)
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if _, ok := lk.Items["skipped"]; ok {
		t.Error("skipped dataset should not have a lock entry")
	}
	if _, ok := lk.Items["wanted"]; !ok {
		t.Error("wanted dataset should have a lock entry")
	}
}

func TestFetch_WriteLockError(t *testing.T) {
	skipUnlessCanDenyRead(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	targetFile := filepath.Join(tmpDir, "target.txt")
	configContent := "version: 1\ndatasets:\n  - id: ds1\n    source:\n      type: mock\n    target: " + targetFile + "\n"
	mustWriteFile(t, configPath, []byte(configContent))

	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, readOnlyDir, 0o755)
	lockPath := filepath.Join(readOnlyDir, "lock.yaml")

	code := Fetch(context.Background(), configPath, lockPath, nil, 1)
	if code != 1 {
		t.Errorf("Fetch() = %d, want 1 when the lockfile can't be written", code)
	}
}

func TestFetchOneDataset_LocalHashErrorAfterFetch(t *testing.T) {
	skipUnlessCanDenyRead(t)
	target := filepath.Join(t.TempDir(), "target.txt")
	ds := Dataset{ID: "ds1", Target: target, Source: registry.Source{Type: "mockunreadable"}}
	store := &lockStore{lk: &Lock{Items: map[string]*LockItem{}}}
	var w strings.Builder

	code := fetchOneDataset(context.Background(), &w, nil, ds, store, time.Now().UTC())
	if code != 0 {
		t.Errorf("fetchOneDataset() = %d, want 0 (fetch succeeded even though hashing failed after)", code)
	}
	if !strings.Contains(w.String(), "local hash after fetch") {
		t.Errorf("expected a warning about hashing after fetch, got %q", w.String())
	}
	chmodOrLog(t, target, 0o644) // let TempDir clean up
}

// --- corrupt lockfile handling ---
//
// Regression tests: Check/Fetch used to discard readLock's error entirely
// (`lk, _ := readLock(lockPath)`), so an existing-but-corrupted lockfile (as opposed to a
// missing one, which readLock handles by returning an empty Lock) left lk as a nil *Lock -
// dereferencing lk.Items on the next line then panicked instead of returning a clean error.

func TestCheck_CorruptLockfileReturnsErrorNotPanic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := "version: 1\ndatasets:\n  - id: ds1\n    source:\n      type: mock\n    target: " + filepath.Join(tmpDir, "t.txt") + "\n"
	mustWriteFile(t, configPath, []byte(configContent))
	lockPath := filepath.Join(tmpDir, "lock.yaml")
	mustWriteFile(t, lockPath, []byte("not: valid: yaml: content:"))

	code := Check(context.Background(), configPath, lockPath, 1)
	if code != 2 {
		t.Errorf("Check() with a corrupt lockfile = %d, want 2", code)
	}
}

func TestFetch_CorruptLockfileReturnsErrorNotPanic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := "version: 1\ndatasets:\n  - id: ds1\n    source:\n      type: mock\n    target: " + filepath.Join(tmpDir, "t.txt") + "\n"
	mustWriteFile(t, configPath, []byte(configContent))
	lockPath := filepath.Join(tmpDir, "lock.yaml")
	mustWriteFile(t, lockPath, []byte("not: valid: yaml: content:"))

	code := Fetch(context.Background(), configPath, lockPath, nil, 1)
	if code != 2 {
		t.Errorf("Fetch() with a corrupt lockfile = %d, want 2", code)
	}
}
