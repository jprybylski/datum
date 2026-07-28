package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

func TestHandler_Name(t *testing.T) {
	h := New()
	if got := h.Name(); got != "file" {
		t.Errorf("Name() = %v, want file", got)
	}
}

func TestHandler_Fingerprint(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "test content for fingerprinting"
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	h := New()

	t.Run("valid file", func(t *testing.T) {
		src := registry.Source{
			Path: testFile,
		}

		fp, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}

		// Check that it returns a SHA256 fingerprint with the correct prefix
		if len(fp) < 7 || fp[:7] != "sha256:" {
			t.Errorf("Fingerprint() = %v, want sha256: prefix", fp)
		}
		// SHA256 hash is 64 hex chars + "sha256:" prefix = 71 chars total
		if len(fp) != 71 {
			t.Errorf("Fingerprint() length = %d, want 71", len(fp))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		src := registry.Source{}
		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for missing path, got nil")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		src := registry.Source{
			Path: filepath.Join(tmpDir, "nonexistent.txt"),
		}
		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for non-existent file, got nil")
		}
	})
}

func TestHandler_Fetch(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	h := New()

	// Create a source file
	srcFile := filepath.Join(tmpDir, "source.txt")
	srcContent := "source file content"
	if err := os.WriteFile(srcFile, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	t.Run("successful fetch", func(t *testing.T) {
		destFile := filepath.Join(tmpDir, "dest", "target.txt")
		src := registry.Source{
			Path: srcFile,
		}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify the file was copied
		gotContent, err := os.ReadFile(destFile)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}
		if string(gotContent) != srcContent {
			t.Errorf("Fetch() copied content = %v, want %v", string(gotContent), srcContent)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		src := registry.Source{}
		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for missing path, got nil")
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		src := registry.Source{
			Path: filepath.Join(tmpDir, "nonexistent.txt"),
		}
		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for non-existent source, got nil")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		destFile := filepath.Join(tmpDir, "deeply", "nested", "path", "target.txt")
		src := registry.Source{
			Path: srcFile,
		}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify the file was created
		if _, err := os.Stat(destFile); err != nil {
			t.Errorf("Fetch() failed to create file at nested path: %v", err)
		}
	})
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestHandler_Fingerprint_Directory(t *testing.T) {
	ctx := context.Background()
	h := New()

	t.Run("directory fingerprint has dirsha256 prefix and is stable", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
		mustWriteFile(t, filepath.Join(srcDir, "sub", "b.txt"), "bbb")

		src := registry.Source{Path: srcDir}
		fp1, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		if len(fp1) < 10 || fp1[:10] != "dirsha256:" {
			t.Errorf("Fingerprint() = %q, want dirsha256: prefix", fp1)
		}

		fp2, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() second call error = %v", err)
		}
		if fp1 != fp2 {
			t.Errorf("Fingerprint() not stable across calls: %q != %q", fp1, fp2)
		}
	})

	t.Run("directory fingerprint changes when contents change", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
		src := registry.Source{Path: srcDir}

		fp1, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}

		mustWriteFile(t, filepath.Join(srcDir, "b.txt"), "bbb")
		fp2, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		if fp1 == fp2 {
			t.Error("Fingerprint() did not change after adding a file to the directory")
		}
	})
}

func TestHandler_Fetch_Directory(t *testing.T) {
	ctx := context.Background()
	h := New()

	t.Run("initial sync recreates the tree under dest", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
		mustWriteFile(t, filepath.Join(srcDir, "sub", "b.txt"), "bbb")

		destDir := filepath.Join(t.TempDir(), "out")
		src := registry.Source{Path: srcDir}
		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		got, err := os.ReadFile(filepath.Join(destDir, "a.txt"))
		if err != nil || string(got) != "aaa" {
			t.Errorf("dest a.txt = %q, %v; want %q, nil", got, err, "aaa")
		}
		got, err = os.ReadFile(filepath.Join(destDir, "sub", "b.txt"))
		if err != nil || string(got) != "bbb" {
			t.Errorf("dest sub/b.txt = %q, %v; want %q, nil", got, err, "bbb")
		}
	})

	t.Run("re-fetch after content change updates the file", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "original")
		destDir := filepath.Join(t.TempDir(), "out")
		src := registry.Source{Path: srcDir}

		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "updated")
		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() second call error = %v", err)
		}

		got, err := os.ReadFile(filepath.Join(destDir, "a.txt"))
		if err != nil || string(got) != "updated" {
			t.Errorf("dest a.txt = %q, %v; want %q, nil", got, err, "updated")
		}
	})

	t.Run("file removed from source is removed from dest", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "keep.txt"), "keep")
		mustWriteFile(t, filepath.Join(srcDir, "removeme.txt"), "gone soon")
		destDir := filepath.Join(t.TempDir(), "out")
		src := registry.Source{Path: srcDir}

		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(destDir, "removeme.txt")); err != nil {
			t.Fatalf("removeme.txt should exist after first fetch: %v", err)
		}

		if err := os.Remove(filepath.Join(srcDir, "removeme.txt")); err != nil {
			t.Fatal(err)
		}
		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() second call error = %v", err)
		}

		if _, err := os.Stat(filepath.Join(destDir, "removeme.txt")); !os.IsNotExist(err) {
			t.Errorf("removeme.txt should have been deleted from dest, stat err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(destDir, "keep.txt")); err != nil {
			t.Errorf("keep.txt should still exist: %v", err)
		}
	})

	t.Run("unrelated pre-existing file in dest is left alone", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "managed.txt"), "managed")
		destDir := t.TempDir()
		// A file datum never wrote, living in the same target directory.
		mustWriteFile(t, filepath.Join(destDir, "unrelated.txt"), "not from datum")

		src := registry.Source{Path: srcDir}
		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		got, err := os.ReadFile(filepath.Join(destDir, "unrelated.txt"))
		if err != nil || string(got) != "not from datum" {
			t.Errorf("unrelated.txt was touched: content = %q, err = %v", got, err)
		}
	})

	t.Run("manifest sidecar is a sibling of dest, not inside it", func(t *testing.T) {
		srcDir := t.TempDir()
		mustWriteFile(t, filepath.Join(srcDir, "a.txt"), "aaa")
		destDir := filepath.Join(t.TempDir(), "out")
		src := registry.Source{Path: srcDir}

		if err := h.Fetch(ctx, src, destDir); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if _, err := os.Stat(destDir + manifestSuffix); err != nil {
			t.Errorf("expected manifest sidecar at %s: %v", destDir+manifestSuffix, err)
		}
		if _, err := os.Stat(filepath.Join(destDir, manifestSuffix)); !os.IsNotExist(err) {
			t.Error("manifest sidecar should not be written inside dest")
		}
	})
}
