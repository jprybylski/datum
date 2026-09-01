package core

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestInitEmptyRejectsInvalidPolicy(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	options := InitOptions{Empty: true, Policy: "sometimes", PolicySet: true}
	if code := Init(configPath, options, strings.NewReader(""), &out, false); code != 2 || !strings.Contains(out.String(), "--policy") {
		t.Fatalf("Init(--empty invalid policy) = %d, output = %q", code, out.String())
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("invalid init created config: %v", err)
	}
}

func TestWriteEmptyConfigReportsWriteFailure(t *testing.T) {
	dir := enterTempDir(t)
	blockingFile := filepath.Join(dir, "blocking")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := writeEmptyConfig(filepath.Join(blockingFile, ".data.yaml"), InitOptions{}, &out)
	if code != 1 || !strings.Contains(out.String(), "write config") {
		t.Fatalf("writeEmptyConfig(blocked path) = %d, output = %q", code, out.String())
	}
}

func TestInitInteractiveWithAllValuesProvided(t *testing.T) {
	dir := enterTempDir(t)
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	options := InitOptions{
		ID: "provided", Type: "file", Source: "source.csv", Target: "data/provided.csv",
		Desc: "Provided", Policy: "log", DescSet: true, PolicySet: true, IgnoreSet: true,
	}
	if code := Init(configPath, options, strings.NewReader(""), &out, true); code != 0 {
		t.Fatalf("Init(prefilled interactive) = %d, output = %q", code, out.String())
	}
	if strings.Contains(out.String(), ": ") {
		t.Fatalf("prefilled interactive init prompted unexpectedly: %q", out.String())
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Datasets[0].Source.Path != "source.csv" || cfg.Defaults.Policy != "log" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestInitRejectsIgnorePreparationFailure(t *testing.T) {
	dir := enterTempDir(t)
	runCommand(t, "git", "init", "--quiet", dir)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "tracked.csv"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, "git", "-C", dir, "add", "data/tracked.csv")
	configPath := filepath.Join(dir, ".data.yaml")
	var out bytes.Buffer
	options := InitOptions{
		ID: "tracked", Type: "file", Source: "source.csv", Target: "data/tracked.csv",
		Policy: "fail", Ignore: true, PolicySet: true, IgnoreSet: true,
	}
	if code := Init(configPath, options, strings.NewReader(""), &out, false); code != 2 || !strings.Contains(out.String(), "already tracked by Git") {
		t.Fatalf("Init(tracked ignored target) = %d, output = %q", code, out.String())
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("failed init created config: %v", err)
	}
}

func TestValidateInitOptions(t *testing.T) {
	valid := InitOptions{ID: "valid_id-1", Type: "http", Source: "https://example.com", Target: "data.csv", Policy: "fail"}
	if err := validateInitOptions(valid); err != nil {
		t.Fatalf("valid options: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*InitOptions)
		want string
	}{
		{"invalid ID", func(o *InitOptions) { o.ID = "bad id" }, "--id"},
		{"invalid type", func(o *InitOptions) { o.Type = "git" }, "--type"},
		{"invalid policy", func(o *InitOptions) { o.Policy = "sometimes" }, "--policy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			test.edit(&options)
			if err := validateInitOptions(options); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateInitOptions() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInitReportsConfigPathInspectionError(t *testing.T) {
	dir := enterTempDir(t)
	var out bytes.Buffer
	code := Init(filepath.Join(dir, "bad\x00path"), InitOptions{Empty: true}, strings.NewReader(""), &out, false)
	if code != 2 || !strings.Contains(out.String(), "inspect config path") {
		t.Fatalf("Init(invalid path) = %d, output = %q", code, out.String())
	}
}

func TestInitInjectedFailurePaths(t *testing.T) {
	options := InitOptions{ID: "example", Type: "file", Source: "source.csv", Target: "data/example.csv", Policy: "fail", PolicySet: true}
	for _, test := range []struct {
		name  string
		setup func()
		want  int
	}{
		{"prepare ignore", func() {
			prepareIgnore = func(*Config) (*ignorePlan, error) { return nil, errors.New("prepare failed") }
		}, 2},
		{"marshal", func() { marshalConfig = func(any) ([]byte, error) { return nil, errors.New("marshal failed") } }, 1},
		{"write", func() { writeConfig = func(string, []byte, os.FileMode) error { return errors.New("write failed") } }, 1},
		{"apply ignore", func() { applyIgnore = func(*ignorePlan) error { return errors.New("apply failed") } }, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			prepareIgnore, marshalConfig, writeConfig, applyIgnore = prepareIgnorePlan, yaml.Marshal, atomicWrite, applyIgnorePlan
			t.Cleanup(func() {
				prepareIgnore, marshalConfig, writeConfig, applyIgnore = prepareIgnorePlan, yaml.Marshal, atomicWrite, applyIgnorePlan
			})
			test.setup()
			var out bytes.Buffer
			if got := Init(filepath.Join(t.TempDir(), "config.yaml"), options, strings.NewReader(""), &out, false); got != test.want {
				t.Fatalf("Init() = %d, output = %q, want %d", got, out.String(), test.want)
			}
		})
	}
}

func TestInitEmptyInjectedFailurePaths(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func()
	}{
		{"marshal", func() { marshalConfig = func(any) ([]byte, error) { return nil, errors.New("marshal failed") } }},
		{"write", func() { writeConfig = func(string, []byte, os.FileMode) error { return errors.New("write failed") } }},
	} {
		t.Run(test.name, func(t *testing.T) {
			marshalConfig, writeConfig = yaml.Marshal, atomicWrite
			t.Cleanup(func() { marshalConfig, writeConfig = yaml.Marshal, atomicWrite })
			test.setup()
			var out bytes.Buffer
			if got := Init(filepath.Join(t.TempDir(), "config.yaml"), InitOptions{Empty: true}, strings.NewReader(""), &out, false); got != 1 {
				t.Fatalf("Init(--empty) = %d, output = %q", got, out.String())
			}
		})
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
