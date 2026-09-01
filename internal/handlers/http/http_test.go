package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
			if _, err := w.Write([]byte(content)); err != nil {
				t.Errorf("test server write error: %v", err)
			}
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

	t.Run("configured headers are sent on HEAD", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Errorf("Authorization = %q, want Bearer secret", got)
			}
			w.Header().Set("ETag", `"authenticated"`)
		}))
		defer server.Close()

		fp, err := New().Fingerprint(ctx, registry.Source{
			URL: server.URL, Headers: map[string]string{"Authorization": "Bearer secret"},
		})
		if err != nil || fp != `etag:"authenticated"` {
			t.Fatalf("Fingerprint() = %q, %v", fp, err)
		}
	})

	t.Run("body implies POST and hashes the response", func(t *testing.T) {
		const response = "generated export"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(body); got != `{"format":"csv"}` {
				t.Errorf("body = %q", got)
			}
			_, _ = io.WriteString(w, response)
		}))
		defer server.Close()

		fp, err := New().Fingerprint(ctx, registry.Source{URL: server.URL, Body: `{"format":"csv"}`})
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(response))
		if want := "sha256:" + hex.EncodeToString(sum[:]); fp != want {
			t.Errorf("fingerprint = %q, want %q", fp, want)
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
			if _, err := w.Write([]byte(content)); err != nil {
				t.Errorf("test server write error: %v", err)
			}
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

	t.Run("POST fetch returns fingerprint without a second request", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			w.Header().Set("ETag", `"post-result"`)
			_, _ = io.WriteString(w, "result")
		}))
		defer server.Close()

		dest := filepath.Join(tmpDir, "post.txt")
		fp, err := New().FetchWithFingerprint(ctx, registry.Source{URL: server.URL, Body: "query"}, dest)
		if err != nil {
			t.Fatal(err)
		}
		if requests != 1 {
			t.Errorf("requests = %d, want 1", requests)
		}
		if fp != `etag:"post-result"` {
			t.Errorf("fingerprint = %q", fp)
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
			if _, err := w.Write([]byte(content)); err != nil {
				t.Errorf("test server write error: %v", err)
			}
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

func TestHandler_RequestAndTransportErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid URLs", func(t *testing.T) {
		h := New()
		if _, err := h.Fingerprint(ctx, registry.Source{URL: "://invalid"}); err == nil {
			t.Fatal("Fingerprint() accepted invalid URL")
		}
		if _, err := h.Fingerprint(ctx, registry.Source{URL: "://invalid", Body: "query"}); err == nil {
			t.Fatal("Fingerprint(POST) accepted invalid URL")
		}
		if _, err := h.FetchWithFingerprint(ctx, registry.Source{URL: "://invalid"}, filepath.Join(t.TempDir(), "out")); err == nil {
			t.Fatal("FetchWithFingerprint() accepted invalid URL")
		}
	})

	t.Run("transport errors", func(t *testing.T) {
		h := &handler{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		})}}
		if _, err := h.Fingerprint(ctx, registry.Source{URL: "https://example.com", Body: "query"}); err == nil || !strings.Contains(err.Error(), "transport failed") {
			t.Fatalf("Fingerprint(transport error) = %v", err)
		}
		if _, err := h.FetchWithFingerprint(ctx, registry.Source{URL: "https://example.com"}, filepath.Join(t.TempDir(), "out")); err == nil || !strings.Contains(err.Error(), "transport failed") {
			t.Fatalf("FetchWithFingerprint(transport error) = %v", err)
		}
	})

	t.Run("HEAD response close error", func(t *testing.T) {
		h := &handler{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"ETag": []string{`"value"`}},
				Body:       closeErrorBody{Reader: strings.NewReader("")},
			}, nil
		})}}
		if _, err := h.Fingerprint(ctx, registry.Source{URL: "https://example.com"}); err == nil || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("Fingerprint(close error) = %v", err)
		}
	})

	t.Run("response read errors", func(t *testing.T) {
		newHandler := func() *handler {
			return &handler{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: readErrorBody{}}, nil
			})}}
		}
		if _, err := newHandler().Fingerprint(ctx, registry.Source{URL: "https://example.com", Body: "query"}); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("Fingerprint(read error) = %v", err)
		}
		dest := filepath.Join(t.TempDir(), "out")
		if _, err := newHandler().FetchWithFingerprint(ctx, registry.Source{URL: "https://example.com"}, dest); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("FetchWithFingerprint(read error) = %v", err)
		}
		if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
			t.Fatalf("failed fetch left temporary file: %v", err)
		}
	})
}

func TestHandler_FetchDestinationErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "content")
	}))
	defer server.Close()
	h := New()
	src := registry.Source{URL: server.URL}

	t.Run("create parent", func(t *testing.T) {
		dir := t.TempDir()
		blockingFile := filepath.Join(dir, "blocking")
		if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.FetchWithFingerprint(context.Background(), src, filepath.Join(blockingFile, "out")); err == nil {
			t.Fatal("FetchWithFingerprint() created a directory beneath a file")
		}
	})

	t.Run("create temporary file", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		if err := os.Mkdir(dest+".tmp", 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := h.FetchWithFingerprint(context.Background(), src, dest); err == nil {
			t.Fatal("FetchWithFingerprint() replaced temporary directory")
		}
	})

	t.Run("rename temporary file", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		if err := os.Mkdir(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.FetchWithFingerprint(context.Background(), src, dest); err == nil {
			t.Fatal("FetchWithFingerprint() renamed over a directory")
		}
		if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
			t.Fatalf("rename failure left temporary file: %v", err)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type closeErrorBody struct{ io.Reader }

func (closeErrorBody) Close() error { return errors.New("close failed") }

type readErrorBody struct{}

func (readErrorBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (readErrorBody) Close() error             { return nil }
