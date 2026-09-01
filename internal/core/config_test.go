package core

import (
	"os"
	"path/filepath"
	"strings"
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

func TestReadConfigExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("DATUM_TEST_HOST", "example.com")
	t.Setenv("DATUM_TEST_SECRET", "token:with#yaml\nsignificance")
	t.Setenv("DATUM_TEST_TARGET", "data/from-env.csv")
	t.Setenv("DATUM_TEST_EMPTY", "")

	configPath := filepath.Join(t.TempDir(), "environment.yaml")
	configContent := `version: 1
datasets:
  - id: env_dataset
    desc: "secret=${DATUM_TEST_SECRET}; empty=${DATUM_TEST_EMPTY}"
    source:
      type: http
      url: "https://${DATUM_TEST_HOST}/data?token=${DATUM_TEST_SECRET}"
      headers:
        Authorization: "Bearer ${DATUM_TEST_SECRET}"
      body: '{"token":"${DATUM_TEST_SECRET}"}'
    target: ${DATUM_TEST_TARGET}
  - id: shell_dataset
    desc: $PLAIN_IS_NOT_EXPANDED and $${LITERAL_REFERENCE}
    source:
      type: command
      fingerprint_cmd: printf '%s' "$PLAIN_IS_FOR_THE_SHELL"
      fetch_cmd: printf '%s' "$PLAIN_IS_FOR_THE_SHELL" > {{dest}}
    target: data/shell.txt
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("readConfig() error = %v", err)
	}
	if got, want := cfg.Datasets[0].Desc, "secret=token:with#yaml\nsignificance; empty="; got != want {
		t.Errorf("Desc = %q, want %q", got, want)
	}
	if got, want := cfg.Datasets[0].Source.URL, "https://example.com/data?token=token:with#yaml\nsignificance"; got != want {
		t.Errorf("Source.URL = %q, want %q", got, want)
	}
	if got, want := cfg.Datasets[0].Source.Headers["Authorization"], "Bearer token:with#yaml\nsignificance"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	if got, want := cfg.Datasets[0].Source.Body, "{\"token\":\"token:with#yaml\nsignificance\"}"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
	if got, want := cfg.Datasets[0].Target, "data/from-env.csv"; got != want {
		t.Errorf("Target = %q, want %q", got, want)
	}
	if got, want := cfg.Datasets[1].Desc, "$PLAIN_IS_NOT_EXPANDED and ${LITERAL_REFERENCE}"; got != want {
		t.Errorf("escaped Desc = %q, want %q", got, want)
	}
	if got := cfg.Datasets[1].Source.FingerprintCmd; !strings.Contains(got, "$PLAIN_IS_FOR_THE_SHELL") {
		t.Errorf("FingerprintCmd = %q, want plain shell variable to remain", got)
	}
}

func TestReadConfigRejectsInvalidEnvironmentReferences(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		want      string
	}{
		{name: "unset", reference: "${DATUM_TEST_DEFINITELY_UNSET_25}", want: `environment variable "DATUM_TEST_DEFINITELY_UNSET_25" is not set`},
		{name: "invalid name", reference: "${NOT-A-NAME}", want: `invalid environment variable name "NOT-A-NAME"`},
		{name: "unterminated", reference: "${UNTERMINATED", want: "unterminated environment reference"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "invalid-environment.yaml")
			configContent := `version: 1
datasets:
  - id: env_dataset
    desc: test
    source:
      type: http
      url: "https://example.com/` + tt.reference + `"
    target: data/test.csv
`
			if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
				t.Fatalf("failed to create test config: %v", err)
			}

			_, err := readConfig(configPath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readConfig() error = %v, want it to contain %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "line 7, column 12") {
				t.Errorf("readConfig() error = %v, want source location", err)
			}
		})
	}
}

func TestReadConfigRejectsExpandedValueWithWrongType(t *testing.T) {
	t.Setenv("DATUM_TEST_VERSION", "not-an-integer")
	configPath := filepath.Join(t.TempDir(), "wrong-type.yaml")
	configContent := `version: "${DATUM_TEST_VERSION}"
datasets: []
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	_, err := readConfig(configPath)
	if err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("readConfig() error = %v, want a type error", err)
	}
}

func TestValidEnvName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "A", want: true},
		{name: "lower_case_2", want: true},
		{name: "_LEADING_UNDERSCORE", want: true},
		{name: "", want: false},
		{name: "2_FAST", want: false},
		{name: "HAS-DASH", want: false},
		{name: "NON_ASCII_é", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validEnvName(tt.name); got != tt.want {
				t.Errorf("validEnvName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestDatasetShouldIgnore(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name       string
		override   *bool
		defaultVal bool
		want       bool
	}{
		{name: "inherits false", want: false},
		{name: "inherits true", defaultVal: true, want: true},
		{name: "enables", override: &trueValue, want: true},
		{name: "disables", override: &falseValue, defaultVal: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := Dataset{Ignore: tt.override}
			if got := ds.ShouldIgnore(tt.defaultVal); got != tt.want {
				t.Errorf("ShouldIgnore(%v) = %v, want %v", tt.defaultVal, got, tt.want)
			}
		})
	}
}
