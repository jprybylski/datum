package file

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

func skipUnlessCanDenyRead(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
}

func chmodOrLog(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Logf("cleanup: failed to chmod %s: %v", path, err)
	}
}

// --- Fingerprint error paths ---

func TestHandler_Fingerprint_UnreadableFile(t *testing.T) {
	skipUnlessCanDenyRead(t)
	f := filepath.Join(t.TempDir(), "f.txt")
	mustWriteFile(t, f, "content")
	if err := os.Chmod(f, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, f, 0o644)

	h := New()
	if _, err := h.Fingerprint(context.Background(), registry.Source{Path: f}); err == nil {
		t.Error("Fingerprint() on an unreadable file expected an error, got nil")
	}
}

func TestHandler_Fingerprint_DirectoryWithUnreadableFile(t *testing.T) {
	skipUnlessCanDenyRead(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	mustWriteFile(t, f, "content")
	if err := os.Chmod(f, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, f, 0o644)

	h := New()
	if _, err := h.Fingerprint(context.Background(), registry.Source{Path: dir}); err == nil {
		t.Error("Fingerprint() on a directory containing an unreadable file expected an error, got nil")
	}
}

// --- fetchFile error paths (via Fetch) ---

func TestHandler_Fetch_SourceStatSucceedsButOpenFails(t *testing.T) {
	skipUnlessCanDenyRead(t)
	src := filepath.Join(t.TempDir(), "src.txt")
	mustWriteFile(t, src, "content")
	if err := os.Chmod(src, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, src, 0o644)

	h := New()
	dest := filepath.Join(t.TempDir(), "dest.txt")
	if err := h.Fetch(context.Background(), registry.Source{Path: src}, dest); err == nil {
		t.Error("Fetch() from an unreadable source expected an error, got nil")
	}
}

func TestHandler_Fetch_DestParentBlockedByFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	mustWriteFile(t, src, "content")

	blocker := filepath.Join(t.TempDir(), "blocker")
	mustWriteFile(t, blocker, "x")
	dest := filepath.Join(blocker, "sub", "dest.txt")

	h := New()
	if err := h.Fetch(context.Background(), registry.Source{Path: src}, dest); err == nil {
		t.Error("Fetch() with a dest parent blocked by a file expected an error, got nil")
	}
}

func TestHandler_Fetch_DestDirNotWritable(t *testing.T) {
	skipUnlessCanDenyRead(t)
	src := filepath.Join(t.TempDir(), "src.txt")
	mustWriteFile(t, src, "content")

	destDir := t.TempDir()
	if err := os.Chmod(destDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, destDir, 0o755)

	h := New()
	dest := filepath.Join(destDir, "dest.txt")
	if err := h.Fetch(context.Background(), registry.Source{Path: src}, dest); err == nil {
		t.Error("Fetch() into a non-writable directory expected an error, got nil")
	}
}

// --- fetchDir error paths ---

func TestHandler_Fetch_Directory_SourceUnreadableSubdir(t *testing.T) {
	skipUnlessCanDenyRead(t)
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
	badSubdir := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(badSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(badSubdir, "b.txt"), "bbb")
	if err := os.Chmod(badSubdir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, badSubdir, 0o755)

	h := New()
	destDir := filepath.Join(t.TempDir(), "out")
	if err := h.Fetch(context.Background(), registry.Source{Path: srcDir}, destDir); err == nil {
		t.Error("Fetch() with an unreadable source subdirectory expected an error, got nil")
	}
}

func TestHandler_Fetch_Directory_DestParentBlockedByFile(t *testing.T) {
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")

	blocker := filepath.Join(t.TempDir(), "blocker")
	mustWriteFile(t, blocker, "x")
	destDir := filepath.Join(blocker, "sub")

	h := New()
	if err := h.Fetch(context.Background(), registry.Source{Path: srcDir}, destDir); err == nil {
		t.Error("Fetch() (directory mode) with dest blocked by a file expected an error, got nil")
	}
}

func TestHandler_Fetch_Directory_OneSourceFileUnreadable(t *testing.T) {
	skipUnlessCanDenyRead(t)
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "good.txt"), "aaa")
	badFile := filepath.Join(srcDir, "bad.txt")
	mustWriteFile(t, badFile, "bbb")
	if err := os.Chmod(badFile, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, badFile, 0o644)

	h := New()
	destDir := filepath.Join(t.TempDir(), "out")
	err := h.Fetch(context.Background(), registry.Source{Path: srcDir}, destDir)
	if err == nil {
		t.Fatal("Fetch() with one unreadable source file expected an error, got nil")
	}
	if want := `copying "bad.txt"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want it to mention %q", err, want)
	}
}

// --- FetchDir conflict / cleanup error paths ---

func TestHandler_FetchDir_ConflictLeavesDestUntouched(t *testing.T) {
	// A relative path already claimed by another dataset must fail the fetch before writing
	// anything, rather than partially writing non-conflicting files.
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "shared.txt"), "mine")
	mustWriteFile(t, filepath.Join(srcDir, "onlymine.txt"), "safe")

	destDir := filepath.Join(t.TempDir(), "out")
	claimed := map[string]bool{"shared.txt": true}

	h := New()
	manifest, err := h.FetchDir(context.Background(), registry.Source{Path: srcDir}, destDir, nil, claimed)
	if err == nil {
		t.Fatal("FetchDir() with a claimed path expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "shared.txt") {
		t.Errorf("error = %v, want it to mention the conflicting path", err)
	}
	if manifest != nil {
		t.Errorf("manifest = %v, want nil on conflict", manifest)
	}
	if _, statErr := os.Stat(destDir); !os.IsNotExist(statErr) {
		t.Errorf("destDir should not have been created on conflict, stat err = %v", statErr)
	}
}

// --- removeEmptyParents error/no-op paths ---

func TestRemoveEmptyParents_NonEmptyDirLeftAlone(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "sub", "sibling.txt"), "still here")

	removeEmptyParents(root, "sub")

	if _, err := os.Stat(filepath.Join(root, "sub")); err != nil {
		t.Errorf("non-empty sub/ should have been left alone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "sibling.txt")); err != nil {
		t.Errorf("sibling.txt should have been left alone: %v", err)
	}
}

func TestRemoveEmptyParents_UnreadableDirIsANoOp(t *testing.T) {
	skipUnlessCanDenyRead(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, sub, 0o755)

	// Should return without panicking; os.ReadDir failing is treated the same as "not empty".
	removeEmptyParents(root, "sub")

	if _, err := os.Stat(sub); err != nil {
		t.Errorf("sub/ should still exist after a failed ReadDir: %v", err)
	}
}

func TestRemoveEmptyParents_RemoveFailsIsANoOp(t *testing.T) {
	skipUnlessCanDenyRead(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// sub/ is empty and readable, but root/ is not writable, so removing sub/ (which requires
	// write permission on its parent) fails even though ReadDir(sub) succeeded.
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, root, 0o755)

	removeEmptyParents(root, "sub")

	if _, err := os.Stat(sub); err != nil {
		t.Errorf("sub/ should still exist after a failed Remove: %v", err)
	}
}
