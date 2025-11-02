package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

// Mock handler that returns a specific fingerprint
type mockHandlerWithFP struct {
	name        string
	fingerprint string
	shouldFail  bool
}

func (m *mockHandlerWithFP) Name() string { return m.name }

func (m *mockHandlerWithFP) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	if m.shouldFail {
		return "", errors.New("fingerprint failed")
	}
	return m.fingerprint, nil
}

func (m *mockHandlerWithFP) Fetch(ctx context.Context, src registry.Source, dest string) error {
	if m.shouldFail {
		return errors.New("fetch failed")
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte("data from "+m.name), 0o644)
}

func init() {
	// Register mock handlers for testing
	registry.Register(&mockHandlerWithFP{name: "primary", fingerprint: "primary-fp", shouldFail: false})
	registry.Register(&mockHandlerWithFP{name: "secondary", fingerprint: "secondary-fp", shouldFail: false})
	registry.Register(&mockHandlerWithFP{name: "failprimary", fingerprint: "", shouldFail: true})
}

func TestMultiSourceConfig(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid single source", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "single.yaml")
		configContent := `version: 1
datasets:
  - id: test1
    source:
      type: mock
    target: /tmp/test.txt
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		cfg, err := readConfig(configPath)
		if err != nil {
			t.Fatalf("readConfig() error = %v", err)
		}

		sources := cfg.Datasets[0].GetSources()
		if len(sources) != 1 {
			t.Errorf("GetSources() returned %d sources, want 1", len(sources))
		}
	})

	t.Run("valid multiple sources", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "multi.yaml")
		configContent := `version: 1
datasets:
  - id: test1
    sources:
      - type: primary
      - type: secondary
    target: /tmp/test.txt
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		cfg, err := readConfig(configPath)
		if err != nil {
			t.Fatalf("readConfig() error = %v", err)
		}

		sources := cfg.Datasets[0].GetSources()
		if len(sources) != 2 {
			t.Errorf("GetSources() returned %d sources, want 2", len(sources))
		}
		if sources[0].Type != "primary" {
			t.Errorf("sources[0].Type = %v, want primary", sources[0].Type)
		}
		if sources[1].Type != "secondary" {
			t.Errorf("sources[1].Type = %v, want secondary", sources[1].Type)
		}
	})

	t.Run("error on both source and sources", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "both.yaml")
		configContent := `version: 1
datasets:
  - id: test1
    source:
      type: mock
    sources:
      - type: primary
    target: /tmp/test.txt
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		_, err := readConfig(configPath)
		if err == nil {
			t.Error("readConfig() should return error when both source and sources are specified")
		}
	})

	t.Run("error on neither source nor sources", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "neither.yaml")
		configContent := `version: 1
datasets:
  - id: test1
    target: /tmp/test.txt
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		_, err := readConfig(configPath)
		if err == nil {
			t.Error("readConfig() should return error when neither source nor sources are specified")
		}
	})
}

func TestMultiSourceFallback(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("fallback to second source on first source failure", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fallback.yaml")
		targetFile := filepath.Join(tmpDir, "fallback_target.txt")
		lockPath := filepath.Join(tmpDir, "fallback_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_fallback
    sources:
      - type: failprimary
      - type: secondary
    target: ` + targetFile + `
    policy: update
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Run Check - should succeed with fallback to secondary
		code := Check(configPath, lockPath)
		if code != 0 {
			t.Errorf("Check() = %d, want 0 (should succeed with fallback)", code)
		}

		// Verify the file was created by the secondary source
		data, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatalf("failed to read target file: %v", err)
		}
		if string(data) != "data from secondary" {
			t.Errorf("target file content = %q, want %q", string(data), "data from secondary")
		}

		// Read lockfile and verify it has the secondary fingerprint
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_fallback"]
		if item == nil {
			t.Fatal("test_fallback item should exist in lockfile")
		}

		if item.RemoteFingerprint != "secondary-fp" {
			t.Errorf("RemoteFingerprint = %v, want secondary-fp", item.RemoteFingerprint)
		}
	})

	t.Run("all sources fail", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "allfail.yaml")
		targetFile := filepath.Join(tmpDir, "allfail_target.txt")
		lockPath := filepath.Join(tmpDir, "allfail_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_allfail
    sources:
      - type: failprimary
      - type: failprimary
    target: ` + targetFile + `
    policy: update
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Run Check - should fail since all sources fail
		code := Check(configPath, lockPath)
		if code != 1 {
			t.Errorf("Check() = %d, want 1 (should fail when all sources fail)", code)
		}

		// Verify the file was not created
		if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
			t.Error("target file should not exist when all sources fail")
		}
	})

	t.Run("first source succeeds immediately", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "firstsuccess.yaml")
		targetFile := filepath.Join(tmpDir, "firstsuccess_target.txt")
		lockPath := filepath.Join(tmpDir, "firstsuccess_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_first
    sources:
      - type: primary
      - type: secondary
    target: ` + targetFile + `
    policy: update
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Run Check - should succeed with first source
		code := Check(configPath, lockPath)
		if code != 0 {
			t.Errorf("Check() = %d, want 0", code)
		}

		// Verify the file was created by the primary source
		data, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatalf("failed to read target file: %v", err)
		}
		if string(data) != "data from primary" {
			t.Errorf("target file content = %q, want %q (should use first source)", string(data), "data from primary")
		}

		// Read lockfile and verify it has the primary fingerprint
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_first"]
		if item == nil {
			t.Fatal("test_first item should exist in lockfile")
		}

		if item.RemoteFingerprint != "primary-fp" {
			t.Errorf("RemoteFingerprint = %v, want primary-fp", item.RemoteFingerprint)
		}
	})
}

func TestMultiSourceFetch(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("fetch with fallback", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fetchfallback.yaml")
		targetFile := filepath.Join(tmpDir, "fetchfallback_target.txt")
		lockPath := filepath.Join(tmpDir, "fetchfallback_lock.yaml")

		configContent := `version: 1
datasets:
  - id: fetch_fallback
    sources:
      - type: failprimary
      - type: secondary
    target: ` + targetFile + `
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Run Fetch - should succeed with fallback to secondary
		code := Fetch(configPath, lockPath, nil)
		if code != 0 {
			t.Errorf("Fetch() = %d, want 0 (should succeed with fallback)", code)
		}

		// Verify the file was created by the secondary source
		data, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatalf("failed to read target file: %v", err)
		}
		if string(data) != "data from secondary" {
			t.Errorf("target file content = %q, want %q", string(data), "data from secondary")
		}

		// Read lockfile and verify it has the secondary fingerprint
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["fetch_fallback"]
		if item == nil {
			t.Fatal("fetch_fallback item should exist in lockfile")
		}

		if item.RemoteFingerprint != "secondary-fp" {
			t.Errorf("RemoteFingerprint = %v, want secondary-fp", item.RemoteFingerprint)
		}
	})

	t.Run("fetch all sources fail", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "fetchallfail.yaml")
		targetFile := filepath.Join(tmpDir, "fetchallfail_target.txt")
		lockPath := filepath.Join(tmpDir, "fetchallfail_lock.yaml")

		configContent := `version: 1
datasets:
  - id: fetch_allfail
    sources:
      - type: failprimary
      - type: failprimary
    target: ` + targetFile + `
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Run Fetch - should fail since all sources fail
		code := Fetch(configPath, lockPath, nil)
		if code != 1 {
			t.Errorf("Fetch() = %d, want 1 (should fail when all sources fail)", code)
		}

		// Read lockfile and verify inaccessible fields are set
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["fetch_allfail"]
		if item == nil {
			t.Fatal("fetch_allfail item should exist in lockfile")
		}

		// Verify InaccessibleAt is set
		if item.InaccessibleAt == nil {
			t.Error("InaccessibleAt should be set when fetch fails")
		}

		// Verify InaccessibleError is set
		if item.InaccessibleError == "" {
			t.Error("InaccessibleError should be set when fetch fails")
		}
	})
}

func TestMultiSourcePolicyBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("fail policy with multiple sources", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "failpolicy.yaml")
		targetFile := filepath.Join(tmpDir, "failpolicy_target.txt")
		lockPath := filepath.Join(tmpDir, "failpolicy_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_failpolicy
    sources:
      - type: failprimary
      - type: secondary
    target: ` + targetFile + `
    policy: fail
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Create a lockfile with an old fingerprint
		lockContent := `version: 1
items:
  test_failpolicy:
    local_sha256: old_hash
    remote_fingerprint: old_fingerprint
`
		os.WriteFile(lockPath, []byte(lockContent), 0o644)

		// Run Check - should fail since fingerprint changed (secondary-fp vs old_fingerprint)
		code := Check(configPath, lockPath)
		if code != 1 {
			t.Errorf("Check() = %d, want 1 (should fail on changed fingerprint)", code)
		}

		// Read lockfile and verify it wasn't updated (fail policy behavior)
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_failpolicy"]
		if item == nil {
			t.Fatal("test_failpolicy item should still exist in lockfile")
		}

		// Verify the fingerprint was NOT updated (fail policy should not update)
		if item.RemoteFingerprint != "old_fingerprint" {
			t.Errorf("RemoteFingerprint = %v, want old_fingerprint (should not update with fail policy)", item.RemoteFingerprint)
		}
	})

	t.Run("log policy with multiple sources", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "logpolicy.yaml")
		targetFile := filepath.Join(tmpDir, "logpolicy_target.txt")
		lockPath := filepath.Join(tmpDir, "logpolicy_lock.yaml")

		configContent := `version: 1
datasets:
  - id: test_logpolicy
    sources:
      - type: failprimary
      - type: secondary
    target: ` + targetFile + `
    policy: log
`
		os.WriteFile(configPath, []byte(configContent), 0o644)

		// Create a lockfile with an old fingerprint
		lockContent := `version: 1
items:
  test_logpolicy:
    local_sha256: old_hash
    remote_fingerprint: old_fingerprint
`
		os.WriteFile(lockPath, []byte(lockContent), 0o644)

		// Run Check - should succeed (log doesn't fail) but reports stale
		code := Check(configPath, lockPath)
		if code != 0 {
			t.Errorf("Check() = %d, want 0 (log policy should not fail)", code)
		}

		// Read lockfile and verify it wasn't updated (log policy behavior)
		lk, err := readLock(lockPath)
		if err != nil {
			t.Fatalf("readLock() error = %v", err)
		}

		item := lk.Items["test_logpolicy"]
		if item == nil {
			t.Fatal("test_logpolicy item should still exist in lockfile")
		}

		// Verify the fingerprint was NOT updated (log policy should not update)
		if item.RemoteFingerprint != "old_fingerprint" {
			t.Errorf("RemoteFingerprint = %v, want old_fingerprint (should not update with log policy)", item.RemoteFingerprint)
		}
	})
}
