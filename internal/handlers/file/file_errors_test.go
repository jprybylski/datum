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

// --- readManifest / writeManifest error paths ---

func TestReadManifest_CorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	mustWriteFile(t, path, "not valid json{{{")

	if _, err := readManifest(path); err == nil {
		t.Error("readManifest() with corrupt JSON expected an error, got nil")
	}
}

func TestReadManifest_MissingFile(t *testing.T) {
	if _, err := readManifest(filepath.Join(t.TempDir(), "nonexistent.json")); err == nil {
		t.Error("readManifest() for a missing file expected an error, got nil")
	}
}

func TestWriteManifest_DirNotWritable(t *testing.T) {
	skipUnlessCanDenyRead(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, dir, 0o755)

	err := writeManifest(filepath.Join(dir, "manifest.json"), []string{"a.txt"})
	if err == nil {
		t.Error("writeManifest() into a non-writable directory expected an error, got nil")
	}
}

// --- directory Fetch with a corrupt pre-existing manifest ---

func TestHandler_Fetch_Directory_CorruptManifestIsIgnored(t *testing.T) {
	// A missing or corrupt manifest just means there's no prior state to diff against - not a
	// fetch failure. This exercises fetchDir's `prevRels, _ := readManifest(...)` swallow path.
	srcDir := t.TempDir()
	mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")

	destDir := filepath.Join(t.TempDir(), "out")
	mustWriteFile(t, destDir+manifestSuffix, "not valid json{{{")

	h := New()
	if err := h.Fetch(context.Background(), registry.Source{Path: srcDir}, destDir); err != nil {
		t.Fatalf("Fetch() with a corrupt pre-existing manifest error = %v, want nil", err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "a.txt"))
	if err != nil || string(got) != "aaa" {
		t.Errorf("a.txt = %q, %v; want %q, nil", got, err, "aaa")
	}
}
