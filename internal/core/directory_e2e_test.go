package core_test

// This file uses the external core_test package (rather than internal package core, like
// engine_test.go) specifically so it can import internal/handlers/file - which itself imports
// internal/core - without creating an import cycle in an internal test file.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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
	if code := core.Check(context.Background(), configPath, lockPath, 1); code != 0 {
		t.Fatalf("Check() = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(targetDir, "sub", "b.txt"))
	if err != nil || string(got) != "bbb" {
		t.Fatalf("target sub/b.txt = %q, %v; want %q, nil", got, err, "bbb")
	}

	// A repeat Check with nothing changed should be a no-op (up-to-date).
	if code := core.Check(context.Background(), configPath, lockPath, 1); code != 0 {
		t.Fatalf("second Check() = %d, want 0", code)
	}

	// Remove a file from the source and confirm the next update-policy Check removes it from
	// target too (issue #8's "deleted upstream contents should not be maintained" requirement).
	if err := os.Remove(filepath.Join(srcDir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if code := core.Check(context.Background(), configPath, lockPath, 1); code != 0 {
		t.Fatalf("Check() after deletion = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("target a.txt should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "sub", "b.txt")); err != nil {
		t.Errorf("target sub/b.txt should still exist: %v", err)
	}

	// Removing sub/b.txt too - the last file in that subdirectory - must remove the now-empty
	// target/sub itself, not just the file (#16).
	if err := os.Remove(filepath.Join(srcDir, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}
	if code := core.Check(context.Background(), configPath, lockPath, 1); code != 0 {
		t.Fatalf("Check() after emptying sub/ = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "sub")); !os.IsNotExist(err) {
		t.Errorf("target sub/ should have been removed once empty, stat err = %v", err)
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Errorf("targetDir itself should still exist: %v", err)
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

	if code := core.Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(targetDir, "only.txt"))
	if err != nil || string(got) != "content" {
		t.Fatalf("target only.txt = %q, %v; want %q, nil", got, err, "content")
	}
}

func readLockYAML(t *testing.T, lockPath string) core.Lock {
	t.Helper()
	b, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile(lockPath) error = %v", err)
	}
	var lk core.Lock
	if err := yaml.Unmarshal(b, &lk); err != nil {
		t.Fatalf("yaml.Unmarshal(lock) error = %v", err)
	}
	return lk
}

// TestFetch_TwoDatasetsShareTarget covers #14 (multiple sources syncing into the same target
// directory, including a shared subdirectory name) and #18 (the manifest that makes this
// possible lives in the lockfile, not a sidecar file next to the target).
func TestFetch_TwoDatasetsShareTarget(t *testing.T) {
	tmpDir := t.TempDir()
	srcA := filepath.Join(tmpDir, "srcA")
	mustWriteFile(t, filepath.Join(srcA, "dir1", "a.txt"), "aaa")
	srcB := filepath.Join(tmpDir, "srcB")
	mustWriteFile(t, filepath.Join(srcB, "dir1", "b.txt"), "bbb")

	targetDir := filepath.Join(tmpDir, "target")
	configPath := filepath.Join(tmpDir, ".data.yaml")
	configContent := `version: 1
datasets:
  - id: ds_a
    source:
      type: file
      path: ` + srcA + `
    target: ` + targetDir + `
  - id: ds_b
    source:
      type: file
      path: ` + srcB + `
    target: ` + targetDir + `
`
	mustWriteFile(t, configPath, configContent)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

	if code := core.Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}

	// Both datasets' subdirectories, sharing the "dir1" name, merged into target without either
	// overwriting the other (#14).
	if got, err := os.ReadFile(filepath.Join(targetDir, "dir1", "a.txt")); err != nil || string(got) != "aaa" {
		t.Errorf("target dir1/a.txt = %q, %v; want %q, nil", got, err, "aaa")
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "dir1", "b.txt")); err != nil || string(got) != "bbb" {
		t.Errorf("target dir1/b.txt = %q, %v; want %q, nil", got, err, "bbb")
	}

	// #18: manifest state lives in the lockfile, not a sidecar file next to target.
	if _, err := os.Stat(targetDir + ".datum-manifest.json"); !os.IsNotExist(err) {
		t.Errorf("no sidecar manifest file should exist, stat err = %v", err)
	}
	lk := readLockYAML(t, lockPath)
	if got := lk.Items["ds_a"].DirPaths; len(got) != 1 || got[0] != "dir1/a.txt" {
		t.Errorf("ds_a DirPaths = %v, want [dir1/a.txt]", got)
	}
	if got := lk.Items["ds_b"].DirPaths; len(got) != 1 || got[0] != "dir1/b.txt" {
		t.Errorf("ds_b DirPaths = %v, want [dir1/b.txt]", got)
	}

	// Removing ds_a's only file and re-fetching only ds_a must remove exactly ds_a's file,
	// without touching ds_b's file living right alongside it in the same target/dir1 (#14's
	// cleanup scoping: ds_a's own prevManifest, not the shared directory's full contents, drives
	// what gets removed). dir1 itself must survive, since ds_b's file still lives there - the
	// empty-directory case (#16) is exercised where a subdirectory has no other owner, in
	// TestHandler_FetchDir_ManifestThreading in the file handler's own test package.
	if err := os.Remove(filepath.Join(srcA, "dir1", "a.txt")); err != nil {
		t.Fatal(err)
	}
	if code := core.Fetch(context.Background(), configPath, lockPath, []string{"ds_a"}, 1); code != 0 {
		t.Fatalf("Fetch(ds_a) after removal = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "dir1", "a.txt")); !os.IsNotExist(err) {
		t.Errorf("target dir1/a.txt should have been removed, stat err = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "dir1", "b.txt")); err != nil || string(got) != "bbb" {
		t.Errorf("target dir1/b.txt should still exist: content = %q, err = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "dir1")); err != nil {
		t.Errorf("target dir1 should still exist (ds_b's file lives there): %v", err)
	}
}

// TestFetch_TwoDatasetsShareTarget_ConflictingPath covers #15: two datasets targeting the same
// directory that both want to write the same relative path must fail the fetch instead of one
// silently clobbering the other.
func TestFetch_TwoDatasetsShareTarget_ConflictingPath(t *testing.T) {
	tmpDir := t.TempDir()
	srcA := filepath.Join(tmpDir, "srcA")
	mustWriteFile(t, filepath.Join(srcA, "dir1", "same.txt"), "from A")
	srcB := filepath.Join(tmpDir, "srcB")
	mustWriteFile(t, filepath.Join(srcB, "dir1", "same.txt"), "from B")

	targetDir := filepath.Join(tmpDir, "target")
	configPath := filepath.Join(tmpDir, ".data.yaml")
	configContent := `version: 1
datasets:
  - id: ds_a
    source:
      type: file
      path: ` + srcA + `
    target: ` + targetDir + `
  - id: ds_b
    source:
      type: file
      path: ` + srcB + `
    target: ` + targetDir + `
`
	mustWriteFile(t, configPath, configContent)
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

	// ds_a fetches first and claims dir1/same.txt; ds_b then conflicts with it.
	code := core.Fetch(context.Background(), configPath, lockPath, nil, 1)
	if code != 1 {
		t.Fatalf("Fetch() = %d, want 1 (ds_b's conflicting path should fail)", code)
	}

	// ds_a's file must be unaffected by ds_b's failed fetch.
	if got, err := os.ReadFile(filepath.Join(targetDir, "dir1", "same.txt")); err != nil || string(got) != "from A" {
		t.Errorf("target dir1/same.txt = %q, %v; want %q, nil (ds_a's file untouched)", got, err, "from A")
	}

	lk := readLockYAML(t, lockPath)
	if item := lk.Items["ds_b"]; item == nil || item.InaccessibleError == "" {
		t.Errorf("ds_b lock item = %+v, want InaccessibleError set describing the conflict", item)
	} else if !strings.Contains(item.InaccessibleError, "dir1/same.txt") {
		t.Errorf("ds_b InaccessibleError = %q, want it to mention the conflicting path", item.InaccessibleError)
	}
}
