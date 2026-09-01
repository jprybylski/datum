package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

// Simple mock handler for testing
type mockHandler struct{}

func (m *mockHandler) Name() string { return "mock" }

func (m *mockHandler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	return "mock-fp", nil
}

func (m *mockHandler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte("mock data"), 0o644)
}

type fingerprintingHandler struct {
	fingerprintCalls int
	fetchCalls       int
	combinedCalls    int
}

func (h *fingerprintingHandler) Name() string { return "fingerprinting" }
func (h *fingerprintingHandler) Fingerprint(context.Context, registry.Source) (string, error) {
	h.fingerprintCalls++
	return "separate-fp", nil
}
func (h *fingerprintingHandler) Fetch(context.Context, registry.Source, string) error {
	h.fetchCalls++
	return nil
}
func (h *fingerprintingHandler) FetchWithFingerprint(_ context.Context, _ registry.Source, dest string) (string, error) {
	h.combinedCalls++
	return "combined-fp", os.WriteFile(dest, []byte("combined"), 0o644)
}

// Mock handler that always fails on fetch
type mockFailHandler struct{}

func (m *mockFailHandler) Name() string { return "mockfail" }

func (m *mockFailHandler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	return "mockfail-fp", nil
}

func (m *mockFailHandler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	return errors.New("simulated network error: connection timeout")
}

func init() {
	registry.Register(&mockHandler{})
	registry.Register(&mockFailHandler{})
}

func TestFetchAttemptUsesCombinedFetchAndFingerprint(t *testing.T) {
	handler := &fingerprintingHandler{}
	dest := filepath.Join(t.TempDir(), "target.txt")
	result, label, err := fetchAttempt(context.Background(), dest, nil, nil)(handler, registry.Source{})
	if err != nil {
		t.Fatal(err)
	}
	if result.fp != "combined-fp" || label != "" {
		t.Fatalf("result = %+v, label = %q", result, label)
	}
	if handler.combinedCalls != 1 || handler.fetchCalls != 0 || handler.fingerprintCalls != 0 {
		t.Fatalf("call counts = combined:%d fetch:%d fingerprint:%d", handler.combinedCalls, handler.fetchCalls, handler.fingerprintCalls)
	}
}

func TestCheckAndFetchAcceptEmptyConfiguration(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".data.yaml")
	lockPath := filepath.Join(dir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	if code := Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
		t.Fatalf("Fetch(empty config) = %d", code)
	}
	if code := Check(context.Background(), configPath, lockPath, 1); code != 0 {
		t.Fatalf("Check(empty config) = %d", code)
	}
}

// mustWriteFile writes test fixture content, failing the test immediately if the write fails
// rather than letting it surface later as a confusing "file not found" from whatever reads it.
func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write test fixture %s: %v", path, err)
	}
}

func TestCheck(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("check with no lockfile", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "config.yaml")
		targetFile := filepath.Join(tmpDir, "target.txt")
		configContent := `version: 1
datasets:
  - id: test1
    source:
      type: mock
    target: ` + targetFile + `
    policy: update
`
		mustWriteFile(t, configPath, []byte(configContent))
		lockPath := filepath.Join(tmpDir, "lock.yaml")

		code := Check(context.Background(), configPath, lockPath, 1)
		if code != 0 {
			t.Errorf("Check() = %d, want 0", code)
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "invalid.yaml")
		lockPath := filepath.Join(tmpDir, "lock2.yaml")
		mustWriteFile(t, configPath, []byte("invalid: yaml: syntax:"))

		code := Check(context.Background(), configPath, lockPath, 1)
		if code != 2 {
			t.Errorf("Check() = %d, want 2", code)
		}
	})

	t.Run("fail policy does not update lockfile", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fail_config.yaml")
		targetFile := filepath.Join(tmpDir, "fail_target.txt")
		lockPath := filepath.Join(tmpDir, "fail_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_fail
    source:
      type: mock
    target: ` + targetFile + `
    policy: fail
`
		mustWriteFile(t, configPath, []byte(configContent))

		// Create a lockfile with an old fingerprint
		lockContent := `version: 1
items:
  test_fail:
    local_sha256: old_hash
    remote_fingerprint: old_fingerprint
`
		mustWriteFile(t, lockPath, []byte(lockContent))

		// Run Check - should fail since fingerprint changed
		code := Check(context.Background(), configPath, lockPath, 1)
		if code != 1 {
			t.Errorf("Check() = %d, want 1 (should fail on changed fingerprint)", code)
		}

		// Read lockfile and verify it wasn't updated
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_fail"]
		if item == nil {
			t.Fatal("test_fail item should still exist in lockfile")
		}

		// Verify the fingerprint was NOT updated
		if item.RemoteFingerprint != "old_fingerprint" {
			t.Errorf("RemoteFingerprint = %v, want old_fingerprint (should not update)", item.RemoteFingerprint)
		}
		if item.LocalSHA256 != "old_hash" {
			t.Errorf("LocalSHA256 = %v, want old_hash (should not update)", item.LocalSHA256)
		}
	})

	t.Run("log policy does not update lockfile", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "log_config.yaml")
		targetFile := filepath.Join(tmpDir, "log_target.txt")
		lockPath := filepath.Join(tmpDir, "log_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_log
    source:
      type: mock
    target: ` + targetFile + `
    policy: log
`
		mustWriteFile(t, configPath, []byte(configContent))

		// Create a lockfile with an old fingerprint
		lockContent := `version: 1
items:
  test_log:
    local_sha256: old_hash
    remote_fingerprint: old_fingerprint
`
		mustWriteFile(t, lockPath, []byte(lockContent))

		// Run Check - should succeed (log doesn't fail)
		code := Check(context.Background(), configPath, lockPath, 1)
		if code != 0 {
			t.Errorf("Check() = %d, want 0 (log policy should not fail)", code)
		}

		// Read lockfile and verify it wasn't updated
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_log"]
		if item == nil {
			t.Fatal("test_log item should still exist in lockfile")
		}

		// Verify the fingerprint was NOT updated
		if item.RemoteFingerprint != "old_fingerprint" {
			t.Errorf("RemoteFingerprint = %v, want old_fingerprint (should not update)", item.RemoteFingerprint)
		}
		if item.LocalSHA256 != "old_hash" {
			t.Errorf("LocalSHA256 = %v, want old_hash (should not update)", item.LocalSHA256)
		}
	})

	t.Run("fetch failure records inaccessible in lockfile", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fail_fetch_config.yaml")
		targetFile := filepath.Join(tmpDir, "fail_fetch_target.txt")
		lockPath := filepath.Join(tmpDir, "fail_fetch_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_fetch_fail
    source:
      type: mockfail
    target: ` + targetFile + `
    policy: update
`
		mustWriteFile(t, configPath, []byte(configContent))

		// Run Check - should fail since fetch fails
		code := Check(context.Background(), configPath, lockPath, 1)
		if code != 1 {
			t.Errorf("Check() = %d, want 1 (should fail on fetch error)", code)
		}

		// Read lockfile and verify inaccessible fields are set
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_fetch_fail"]
		if item == nil {
			t.Fatal("test_fetch_fail item should exist in lockfile")
		}

		// Verify InaccessibleAt is set
		if item.InaccessibleAt == nil {
			t.Error("InaccessibleAt should be set when fetch fails")
		}

		// Verify InaccessibleError contains the error message
		if item.InaccessibleError != "simulated network error: connection timeout" {
			t.Errorf("InaccessibleError = %v, want 'simulated network error: connection timeout'", item.InaccessibleError)
		}
	})
}

// TestCheckFetch_OrphanedLockEntryPersists covers a dataset being removed from .data.yaml while
// its lockfile entry survives. Neither Check nor Fetch ever deletes lockfile map entries - they
// only read/mutate the entries for datasets still present in cfg.Datasets and write the whole map
// back - so an orphaned entry (a dataset id with no corresponding config entry) is preserved
// as-is indefinitely, without a config change ever pruning history the user might still want. This
// is what makes `datum delete` + `datum undelete` safe to use across a config edit, and is the
// same property a future `datum unlock` would need to explicitly opt out of.
func TestCheckFetch_OrphanedLockEntryPersists(t *testing.T) {
	tmpDir := t.TempDir()
	target1 := filepath.Join(tmpDir, "orphan_target.txt")
	target2 := filepath.Join(tmpDir, "kept_target.txt")
	configPath := filepath.Join(tmpDir, "config.yaml")
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	twoDatasetConfig := `version: 1
datasets:
  - id: orphan_ds
    source:
      type: mock
    target: ` + target1 + `
    policy: update
  - id: kept_ds
    source:
      type: mock
    target: ` + target2 + `
    policy: update
`
	mustWriteFile(t, configPath, []byte(twoDatasetConfig))

	if code := Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
		t.Fatalf("initial Fetch() = %d, want 0", code)
	}

	lkBefore, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	orphanBefore := lkBefore.Items["orphan_ds"]
	if orphanBefore == nil {
		t.Fatal("orphan_ds should have a lock entry after the initial fetch")
	}

	// Rewrite the config with orphan_ds removed entirely, as if the user deleted it from
	// .data.yaml by hand - datum should never see this dataset again.
	oneDatasetConfig := `version: 1
datasets:
  - id: kept_ds
    source:
      type: mock
    target: ` + target2 + `
    policy: update
`
	mustWriteFile(t, configPath, []byte(oneDatasetConfig))

	t.Run("check does not prune the orphaned entry", func(t *testing.T) {
		if code := Check(context.Background(), configPath, lockPath, 1); code != 0 {
			t.Fatalf("Check() after config removal = %d, want 0", code)
		}
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}
		orphan := lk.Items["orphan_ds"]
		if orphan == nil {
			t.Fatal("orphan_ds's lock entry should still exist after Check(), even though it's no longer in .data.yaml")
		}
		if orphan.RemoteFingerprint != orphanBefore.RemoteFingerprint || orphan.LocalSHA256 != orphanBefore.LocalSHA256 {
			t.Errorf("orphan_ds lock entry changed = %+v, want unchanged from %+v", orphan, orphanBefore)
		}
		if lk.Items["kept_ds"] == nil {
			t.Error("kept_ds's lock entry should still exist")
		}
	})

	t.Run("fetch does not prune the orphaned entry", func(t *testing.T) {
		if code := Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
			t.Fatalf("Fetch() after config removal = %d, want 0", code)
		}
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}
		if lk.Items["orphan_ds"] == nil {
			t.Fatal("orphan_ds's lock entry should still exist after Fetch(), even though it's no longer in .data.yaml")
		}
	})

	// The orphaned target file itself is untouched too - Check/Fetch never delete local files for
	// a dataset id that simply isn't mentioned in the config at all (contrast with `datum delete`,
	// which is the explicit, confirmed way to remove them).
	if _, err := os.Stat(target1); err != nil {
		t.Errorf("orphan_ds's target file should be untouched: %v", err)
	}
}

func TestFetch(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("fetch all datasets", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fetchcfg.yaml")
		targetFile := filepath.Join(tmpDir, "ftarget.txt")
		configContent := `version: 1
datasets:
  - id: fetch1
    source:
      type: mock
    target: ` + targetFile + `
`
		mustWriteFile(t, configPath, []byte(configContent))
		lockPath := filepath.Join(tmpDir, "fetchlock.yaml")

		code := Fetch(context.Background(), configPath, lockPath, nil, 1)
		if code != 0 {
			t.Errorf("Fetch() = %d, want 0", code)
		}

		if _, err := os.Stat(targetFile); err != nil {
			t.Errorf("target file not created: %v", err)
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "finvalid.yaml")
		lockPath := filepath.Join(tmpDir, "flock.yaml")
		mustWriteFile(t, configPath, []byte("bad: yaml: syntax:"))

		code := Fetch(context.Background(), configPath, lockPath, nil, 1)
		if code != 2 {
			t.Errorf("Fetch() = %d, want 2", code)
		}
	})

	t.Run("fetch failure records inaccessible in lockfile", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fetch_fail_config.yaml")
		targetFile := filepath.Join(tmpDir, "fetch_fail_target.txt")
		lockPath := filepath.Join(tmpDir, "fetch_fail_lock.yaml")

		configContent := `version: 1
datasets:
  - id: fetch_fail_test
    source:
      type: mockfail
    target: ` + targetFile + `
`
		mustWriteFile(t, configPath, []byte(configContent))

		// Run Fetch - should fail since fetch fails
		code := Fetch(context.Background(), configPath, lockPath, nil, 1)
		if code != 1 {
			t.Errorf("Fetch() = %d, want 1 (should fail on fetch error)", code)
		}

		// Read lockfile and verify inaccessible fields are set
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["fetch_fail_test"]
		if item == nil {
			t.Fatal("fetch_fail_test item should exist in lockfile")
		}

		// Verify InaccessibleAt is set
		if item.InaccessibleAt == nil {
			t.Error("InaccessibleAt should be set when fetch fails")
		}

		// Verify InaccessibleError contains the error message
		if item.InaccessibleError != "simulated network error: connection timeout" {
			t.Errorf("InaccessibleError = %v, want 'simulated network error: connection timeout'", item.InaccessibleError)
		}
	})
}
