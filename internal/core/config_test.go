package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "valid.yaml")
		configContent := `version: 1
defaults:
  policy: fail
  algo: sha256

datasets:
  - id: test_dataset
    desc: Test dataset
    source:
      type: http
      url: https://example.com/data.csv
    target: data/test.csv
    policy: update
`
		if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
			t.Fatalf("failed to create test config: %v", err)
		}

		cfg, err := readConfig(configPath)
		if err != nil {
			t.Fatalf("readConfig() error = %v", err)
		}

		if cfg.Version != 1 {
			t.Errorf("Version = %v, want 1", cfg.Version)
		}
		if cfg.Defaults.Policy != "fail" {
			t.Errorf("Defaults.Policy = %v, want fail", cfg.Defaults.Policy)
		}
		if cfg.Defaults.Algo != "sha256" {
			t.Errorf("Defaults.Algo = %v, want sha256", cfg.Defaults.Algo)
		}
		if len(cfg.Datasets) != 1 {
			t.Fatalf("len(Datasets) = %v, want 1", len(cfg.Datasets))
		}
		if cfg.Datasets[0].ID != "test_dataset" {
			t.Errorf("Datasets[0].ID = %v, want test_dataset", cfg.Datasets[0].ID)
		}
		if cfg.Datasets[0].Source.Type != "http" {
			t.Errorf("Datasets[0].Source.Type = %v, want http", cfg.Datasets[0].Source.Type)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := readConfig(filepath.Join(tmpDir, "nonexistent.yaml"))
		if err == nil {
			t.Error("readConfig() expected error for non-existent file, got nil")
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		invalidPath := filepath.Join(tmpDir, "invalid.yaml")
		invalidContent := "this is not: valid: yaml: content:"
		if err := os.WriteFile(invalidPath, []byte(invalidContent), 0o644); err != nil {
			t.Fatalf("failed to create invalid config: %v", err)
		}

		_, err := readConfig(invalidPath)
		if err == nil {
			t.Error("readConfig() expected error for invalid YAML, got nil")
		}
	})
}
