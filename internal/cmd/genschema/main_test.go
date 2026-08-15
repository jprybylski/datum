package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesSchemaSource(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "schema.json")
	output := filepath.Join(dir, "schema_generated.go")
	if err := os.WriteFile(input, []byte("{\n  \"title\": \"test\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run(input, output, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Code generated", "package datum", `const configSchema = "{\n`} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("generated source missing %q:\n%s", want, generated)
		}
	}
}

func TestRunReportsReadError(t *testing.T) {
	var stderr bytes.Buffer
	code := run(filepath.Join(t.TempDir(), "missing.json"), filepath.Join(t.TempDir(), "out.go"), &stderr)
	if code != 1 || stderr.Len() == 0 {
		t.Fatalf("run(missing input) = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunReportsWriteError(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(input, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := run(input, filepath.Join(dir, "missing", "out.go"), &stderr)
	if code != 1 || stderr.Len() == 0 {
		t.Fatalf("run(unwritable output) = %d, stderr = %q", code, stderr.String())
	}
}

func TestMain(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	originalExit := exitProcess
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
		exitProcess = originalExit
	})

	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "data-schema.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		main()
		if _, err := os.Stat(filepath.Join(dir, "schema_generated.go")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		exitCode := 0
		exitProcess = func(code int) { exitCode = code }
		main()
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1", exitCode)
		}
	})
}
