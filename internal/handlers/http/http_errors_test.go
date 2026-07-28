package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

func skipUnlessCanDenyWrite(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
}

// closedServerURL starts and immediately closes a test server, returning a URL that will
// reliably refuse connections - used to exercise the h.client.Do(...) error branches, since both
// the HEAD and GET requests fail identically against a server that's no longer listening.
func closedServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

func TestHandler_Fingerprint_ConnectionRefused(t *testing.T) {
	h := New()
	_, err := h.Fingerprint(context.Background(), registry.Source{URL: closedServerURL(t)})
	if err == nil {
		t.Error("Fingerprint() against an unreachable server expected an error, got nil")
	}
}

func TestHandler_Fetch_ConnectionRefused(t *testing.T) {
	h := New()
	dest := filepath.Join(t.TempDir(), "out.txt")
	err := h.Fetch(context.Background(), registry.Source{URL: closedServerURL(t)}, dest)
	if err == nil {
		t.Error("Fetch() against an unreachable server expected an error, got nil")
	}
}

// truncatedBodyServer declares a larger Content-Length than it actually writes, then closes the
// connection - the client's body reader knows how many bytes were promised and returns
// io.ErrUnexpectedEOF partway through, exercising the io.Copy error branches in both
// Fingerprint's GET fallback and Fetch.
func truncatedBodyServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			// No ETag/Last-Modified/Content-Length in the HEAD response, so Fingerprint falls
			// through to the GET-and-hash path.
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
}

func TestHandler_Fingerprint_TruncatedBody(t *testing.T) {
	srv := truncatedBodyServer()
	defer srv.Close()

	h := New()
	_, err := h.Fingerprint(context.Background(), registry.Source{URL: srv.URL})
	if err == nil {
		t.Error("Fingerprint() with a truncated body expected an error, got nil")
	}
}

func TestHandler_Fetch_TruncatedBody(t *testing.T) {
	srv := truncatedBodyServer()
	defer srv.Close()

	h := New()
	dest := filepath.Join(t.TempDir(), "out.txt")
	err := h.Fetch(context.Background(), registry.Source{URL: srv.URL}, dest)
	if err == nil {
		t.Error("Fetch() with a truncated body expected an error, got nil")
	}
}

func TestHandler_Fetch_DestParentBlockedByFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(blocker, "sub", "out.txt")

	h := New()
	if err := h.Fetch(context.Background(), registry.Source{URL: srv.URL}, dest); err == nil {
		t.Error("Fetch() with dest parent blocked by a file expected an error, got nil")
	}
}

func TestHandler_Fetch_DestDirNotWritable(t *testing.T) {
	skipUnlessCanDenyWrite(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	destDir := t.TempDir()
	if err := os.Chmod(destDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(destDir, 0o755); err != nil {
			t.Logf("cleanup: failed to chmod: %v", err)
		}
	}()

	h := New()
	dest := filepath.Join(destDir, "out.txt")
	if err := h.Fetch(context.Background(), registry.Source{URL: srv.URL}, dest); err == nil {
		t.Error("Fetch() into a non-writable directory expected an error, got nil")
	}
}
