package core_test

// This file uses the external core_test package (rather than internal package core, like
// engine_test.go) specifically so it can import internal/handlers/file - which itself imports
// internal/core - without creating an import cycle in an internal test file.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jprybylski/datum/internal/core"
	_ "github.com/jprybylski/datum/internal/handlers/file"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestCheckFetch_DirectorySource(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
	mustWriteFile(t, filepath.Join(srcDir, "sub", "b.txt"), "bbb")

	targetDir := filepath.Join(tmpDir, "target")
	configPath := filepath.Join(tmpDir, ".data.yaml")
	configContent := `version: 1
datasets:
  - id: dir_dataset
    source:
      type: file
      path: ` + srcDir + `
    target: ` + targetDir + `
    policy: update
`
	mustWriteFile(t, configPath, configContent)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

	// First Check with update policy should fetch the whole directory.
	if code := core.Check(configPath, lockPath); code != 0 {
		t.Fatalf("Check() = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(targetDir, "sub", "b.txt"))
	if err != nil || string(got) != "bbb" {
		t.Fatalf("target sub/b.txt = %q, %v; want %q, nil", got, err, "bbb")
	}

	// A repeat Check with nothing changed should be a no-op (up-to-date).
	if code := core.Check(configPath, lockPath); code != 0 {
		t.Fatalf("second Check() = %d, want 0", code)
	}

	// Remove a file from the source and confirm the next update-policy Check removes it from
	// target too (issue #8's "deleted upstream contents should not be maintained" requirement).
	if err := os.Remove(filepath.Join(srcDir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if code := core.Check(configPath, lockPath); code != 0 {
		t.Fatalf("Check() after deletion = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("target a.txt should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "sub", "b.txt")); err != nil {
		t.Errorf("target sub/b.txt should still exist: %v", err)
	}
}

func TestFetch_DirectorySource(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	mustWriteFile(t, filepath.Join(srcDir, "only.txt"), "content")

	targetDir := filepath.Join(tmpDir, "target")
	configPath := filepath.Join(tmpDir, ".data.yaml")
	configContent := `version: 1
datasets:
  - id: dir_dataset
    source:
      type: file
      path: ` + srcDir + `
    target: ` + targetDir + `
`
	mustWriteFile(t, configPath, configContent)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

	if code := core.Fetch(configPath, lockPath, nil); code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(targetDir, "only.txt"))
	if err != nil || string(got) != "content" {
		t.Fatalf("target only.txt = %q, %v; want %q, nil", got, err, "content")
	}
}
