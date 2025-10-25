package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

func TestHandler_Name(t *testing.T) {
	h := New()
	if got := h.Name(); got != "http" {
		t.Errorf("Name() = %v, want http", got)
	}
}

func TestHandler_Fingerprint(t *testing.T) {
	ctx := context.Background()

	t.Run("ETag fingerprint", func(t *testing.T) {
		// Create a mock server that returns an ETag
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodHead {
				w.Header().Set("ETag", `"abc123"`)
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		h := New()
		src := registry.Source{URL: server.URL}

		fp, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		if fp != `etag:"abc123"` {
			t.Errorf("Fingerprint() = %v, want etag:\"abc123\"", fp)
		}
	})

	t.Run("Last-Modified fingerprint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodHead {
				w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
				w.Header().Set("Content-Length", "1234")
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		h := New()
		src := registry.Source{URL: server.URL}

		fp, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		if fp != "lm:Wed, 21 Oct 2015 07:28:00 GMT|len:1234" {
			t.Errorf("Fingerprint() = %v, want Last-Modified fingerprint", fp)
		}
	})

	t.Run("SHA256 fallback fingerprint", func(t *testing.T) {
		content := "test content"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// For HEAD requests, return an error to force GET with SHA256 hashing
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(content))
		}))
		defer server.Close()

		h := New()
		src := registry.Source{URL: server.URL}

		fp, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		// Check that it starts with sha256:
		if len(fp) < 7 || fp[:7] != "sha256:" {
			t.Errorf("Fingerprint() = %v, want sha256: prefix", fp)
		}
	})

	t.Run("missing URL", func(t *testing.T) {
		h := New()
		src := registry.Source{}

		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for missing URL, got nil")
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		h := New()
		src := registry.Source{URL: server.URL}

		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for 404, got nil")
		}
	})
}

func TestHandler_Fetch(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	t.Run("successful fetch", func(t *testing.T) {
		content := "downloaded content"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(content))
		}))
		defer server.Close()

		h := New()
		destFile := filepath.Join(tmpDir, "test", "output.txt")
		src := registry.Source{URL: server.URL}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify the file was created
		gotContent, err := os.ReadFile(destFile)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}
		if string(gotContent) != content {
			t.Errorf("Fetch() content = %v, want %v", string(gotContent), content)
		}
	})

	t.Run("missing URL", func(t *testing.T) {
		h := New()
		src := registry.Source{}

		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for missing URL, got nil")
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		h := New()
		src := registry.Source{URL: server.URL}

		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for HTTP 500, got nil")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		content := "test"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(content))
		}))
		defer server.Close()

		h := New()
		destFile := filepath.Join(tmpDir, "deep", "nested", "path", "file.txt")
		src := registry.Source{URL: server.URL}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(destFile); err != nil {
			t.Errorf("Fetch() failed to create nested file: %v", err)
		}
	})
}
