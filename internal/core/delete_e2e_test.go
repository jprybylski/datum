package core_test

// External core_test package (see directory_e2e_test.go's header comment) so this can import
// internal/handlers/file to exercise `datum delete` against a real directory-synced dataset.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jprybylski/datum/internal/core"
	_ "github.com/jprybylski/datum/internal/handlers/file"
)

func TestDelete_DirectorySource_RemovesAllFilesAndEmptyDirs(t *testing.T) {
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

	if code := core.Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "sub", "b.txt")); err != nil {
		t.Fatalf("target sub/b.txt should exist before delete: %v", err)
	}

	if code := core.Delete(configPath, lockPath, []string{"dir_dataset"}, true, nil, discard{}); code != 0 {
		t.Fatalf("Delete() = %d, want 0", code)
	}

	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Errorf("target dir should have been removed entirely (not shared, now empty), stat err = %v", err)
	}
	if _, err := os.Stat(srcDir); err != nil {
		t.Errorf("source dir must be untouched by delete: %v", err)
	}

	lk := readLockYAML(t, lockPath)
	item := lk.Items["dir_dataset"]
	if item == nil || !item.Deleted {
		t.Fatalf("lock item = %+v, want Deleted=true", item)
	}
}

// TestDelete_DirectorySource_SharedTarget_KeepsOtherDatasetsFiles covers the case from issue #17
// where deleting one dataset must not disturb another dataset's files living in the same target
// directory (see TestFetch_TwoDatasetsShareTarget in directory_e2e_test.go for the fetch-side
// version of this same sharing rule).
func TestDelete_DirectorySource_SharedTarget_KeepsOtherDatasetsFiles(t *testing.T) {
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

	if code := core.Delete(configPath, lockPath, []string{"ds_a"}, true, nil, discard{}); code != 0 {
		t.Fatalf("Delete(ds_a) = %d, want 0", code)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "dir1", "a.txt")); !os.IsNotExist(err) {
		t.Errorf("ds_a's file should have been removed, stat err = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "dir1", "b.txt")); err != nil || string(got) != "bbb" {
		t.Errorf("ds_b's file should survive ds_a's delete: content = %q, err = %v", got, err)
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Errorf("shared target dir should survive since ds_b still targets it: %v", err)
	}

	lk := readLockYAML(t, lockPath)
	if item := lk.Items["ds_a"]; item == nil || !item.Deleted {
		t.Errorf("ds_a lock item = %+v, want Deleted=true", item)
	}
	if item := lk.Items["ds_b"]; item == nil || item.Deleted {
		t.Errorf("ds_b lock item = %+v, want Deleted=false (untouched)", item)
	}
}

// TestDelete_DirectorySource_RemovalError covers deleteDatasetFiles' error path: a file that
// can't be removed (parent directory made read-only) must surface as an [ERR ] line and exit 1,
// distinct from the file simply already being gone (which is fine and silently skipped).
func TestDelete_DirectorySource_RemovalError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}

	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")

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

	if err := os.Chmod(targetDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(targetDir, 0o755); err != nil {
			t.Logf("cleanup: failed to chmod %s: %v", targetDir, err)
		}
	}()

	var out bytes.Buffer
	code := core.Delete(configPath, lockPath, []string{"dir_dataset"}, true, nil, &out)
	if code != 1 {
		t.Errorf("Delete() with unremovable file = %d, want 1; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "[ERR ]") {
		t.Errorf("output = %q, want an [ERR ] line", out.String())
	}
}

// TestDelete_DirectorySource_EmptyParentRemovalFails covers removeEmptyParents' own error path:
// when a now-empty subdirectory can't be removed (its *parent* made read-only, so the rmdir itself
// is denied - distinct from TestDelete_DirectorySource_RemovalError, which denies removing the
// file), the empty directory is silently left behind rather than failing the whole delete - Delete
// still succeeds and still removes the file itself.
func TestDelete_DirectorySource_EmptyParentRemovalFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}

	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	mustWriteFile(t, filepath.Join(srcDir, "sub1", "sub2", "a.txt"), "aaa")

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

	// Deny write on sub1 so rmdir-ing the now-empty sub2 (removeEmptyParents' job) fails, even
	// though removing a.txt itself (which only needs write on sub2, untouched here) still works.
	sub1 := filepath.Join(targetDir, "sub1")
	if err := os.Chmod(sub1, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(sub1, 0o755); err != nil {
			t.Logf("cleanup: failed to chmod %s: %v", sub1, err)
		}
	}()

	var out bytes.Buffer
	code := core.Delete(configPath, lockPath, []string{"dir_dataset"}, true, nil, &out)
	if code != 0 {
		t.Fatalf("Delete() = %d, want 0 (file removal itself should still succeed); output: %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(sub1, "sub2", "a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub1, "sub2")); err != nil {
		t.Errorf("sub2 should survive (its removal was denied), but got stat err = %v", err)
	}
}

// discard implements io.Writer, silently dropping Delete's progress/confirmation output for tests
// that only care about the resulting filesystem/lockfile state.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
