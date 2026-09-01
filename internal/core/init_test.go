package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitNoninteractive(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	code := Init(configPath, InitOptions{
		ID: "example", Type: "http", Source: "https://example.com/data.csv",
		Target: "data/example.csv", Desc: "Example data", Policy: "update", Ignore: true,
		DescSet: true, PolicySet: true, IgnoreSet: true,
	}, strings.NewReader(""), &out, false)
	if code != 0 {
		t.Fatalf("Init() = %d, output = %q", code, out.String())
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Policy != "update" || !cfg.Defaults.Ignore {
		t.Errorf("defaults = %+v", cfg.Defaults)
	}
	if got := cfg.Datasets[0]; got.ID != "example" || got.Desc != "Example data" || got.Source.URL != "https://example.com/data.csv" {
		t.Errorf("dataset = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".data.lock.yaml")); !os.IsNotExist(err) {
		t.Errorf("init created a lockfile: %v", err)
	}
}

func TestInitInteractiveDefaults(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, "custom.yaml")
	input := strings.NewReader("\nyes\nyes\nsample\nfile\nsource.csv\ndata/sample.csv\n\n")
	var out bytes.Buffer
	if code := Init(configPath, InitOptions{}, input, &out, true); code != 0 {
		t.Fatalf("Init() = %d, output = %q", code, out.String())
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Datasets[0]; got.Desc != "sample" || got.Source.Path != "source.csv" {
		t.Errorf("dataset = %+v", got)
	}
	if cfg.Defaults.Policy != "fail" || !cfg.Defaults.Ignore {
		t.Errorf("defaults = %+v", cfg.Defaults)
	}
}

func TestInitInteractiveDefaultsToNoDatasets(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	if code := Init(configPath, InitOptions{}, strings.NewReader("\n\n\n"), &out, true); code != 0 {
		t.Fatalf("Init() = %d, output = %q", code, out.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil || string(content) != "version: 1\ndatasets: []\n" {
		t.Fatalf("interactive empty config = %q, %v", content, err)
	}
	if !strings.Contains(out.String(), "Add an initial dataset? (y/N) [n]") {
		t.Fatalf("interactive output did not ask about a dataset: %q", out.String())
	}
}

func TestInitEmpty(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	if code := Init(configPath, InitOptions{Empty: true}, strings.NewReader(""), &out, false); code != 0 {
		t.Fatalf("Init(--empty) = %d, output = %q", code, out.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "version: 1\ndatasets: []\n"; got != want {
		t.Fatalf("empty config = %q, want %q", got, want)
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("readConfig(empty scaffold): %v", err)
	}
	if len(cfg.Datasets) != 0 || cfg.Defaults.Policy != "fail" || cfg.Defaults.Algo != "sha256" {
		t.Fatalf("parsed empty config = %+v", cfg)
	}
}

func TestInitEmptyRejectsDatasetFlags(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	code := Init(configPath, InitOptions{Empty: true, ID: "extra"}, strings.NewReader(""), &out, false)
	if code != 2 || !strings.Contains(out.String(), "cannot be combined") {
		t.Fatalf("Init(--empty --id) = %d, output = %q", code, out.String())
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("invalid empty init created config: %v", err)
	}
}

func TestInitEmptyCanSetDefaults(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	options := InitOptions{Empty: true, Policy: "update", Ignore: true, PolicySet: true, IgnoreSet: true}
	if code := Init(configPath, options, strings.NewReader(""), &out, false); code != 0 {
		t.Fatalf("Init(--empty with defaults) = %d, output = %q", code, out.String())
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Datasets) != 0 || cfg.Defaults.Policy != "update" || !cfg.Defaults.Ignore {
		t.Fatalf("empty config defaults = %+v, datasets = %d", cfg.Defaults, len(cfg.Datasets))
	}
}

func TestInitValidationAndExistingFile(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	if code := Init(configPath, InitOptions{}, strings.NewReader(""), &out, false); code != 2 || !strings.Contains(out.String(), "--id, --type, --source, --target") {
		t.Fatalf("missing Init() = %d, output = %q", code, out.String())
	}
	out.Reset()
	if err := os.WriteFile(configPath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Init(configPath, InitOptions{}, strings.NewReader(""), &out, false); code != 2 || !strings.Contains(out.String(), "already exists") {
		t.Fatalf("existing Init() = %d, output = %q", code, out.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil || string(content) != "existing" {
		t.Fatalf("existing config changed: %q, %v", content, err)
	}
}

func enterTempDir(t *testing.T) string {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	return dir
}
