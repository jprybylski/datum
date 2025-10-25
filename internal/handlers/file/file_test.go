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
