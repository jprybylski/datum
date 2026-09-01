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
	input := strings.NewReader("sample\nfile\nsource.csv\ndata/sample.csv\n\n\n yes \n")
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
