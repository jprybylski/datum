package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"example.com/datum/internal/registry"
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

func init() {
	registry.Register(&mockHandler{})
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
		os.WriteFile(configPath, []byte(configContent), 0o644)
		lockPath := filepath.Join(tmpDir, "lock.yaml")

		code := Check(configPath, lockPath)
		if code != 0 {
			t.Errorf("Check() = %d, want 0", code)
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "invalid.yaml")
		lockPath := filepath.Join(tmpDir, "lock2.yaml")
		os.WriteFile(configPath, []byte("invalid: yaml: syntax:"), 0o644)

		code := Check(configPath, lockPath)
		if code != 2 {
			t.Errorf("Check() = %d, want 2", code)
		}
	})
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
		os.WriteFile(configPath, []byte(configContent), 0o644)
		lockPath := filepath.Join(tmpDir, "fetchlock.yaml")

		code := Fetch(configPath, lockPath, nil)
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
		os.WriteFile(configPath, []byte("bad: yaml: syntax:"), 0o644)

		code := Fetch(configPath, lockPath, nil)
		if code != 2 {
			t.Errorf("Fetch() = %d, want 2", code)
		}
	})
}
