package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

// recorder is a server that records the requests it gets.
type recorder struct {
	*httptest.Server
	mu   sync.Mutex
	seen []http.Header
}

func newRecorder(tls bool, reply http.HandlerFunc) *recorder {
	r := &recorder{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.seen = append(r.seen, req.Header.Clone())
		r.mu.Unlock()
		reply(w, req)
	})
	if tls {
		r.Server = httptest.NewTLSServer(handler)
	} else {
		r.Server = httptest.NewServer(handler)
	}
	return r
}

func (r *recorder) requests() []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("ETag", `"v1"`)
	_, _ = w.Write([]byte("data"))
}

func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusFound) }
}

// handlerFor builds the production handler (with its redirect policy) trusting the test TLS cert.
func handlerFor(tlsServer *httptest.Server) *handler {
	h := New()
	if tlsServer != nil {
		h.client.Transport = tlsServer.Client().Transport
	}
	return h
}

func TestRedirect_FromHTTPSToHTTPIsRefused(t *testing.T) {
	plain := newRecorder(false, ok)
	defer plain.Close()
	secure := newRecorder(true, redirectTo(plain.URL+"/a.csv"))
	defer secure.Close()

	h := handlerFor(secure.Server)
	src := registry.Source{URL: secure.URL + "/a.csv?token=t", Headers: map[string]string{"Authorization": "Bearer t"}}
	if _, err := h.Fingerprint(context.Background(), src); err == nil {
		// HEAD fails, then GET fails: both must be refused.
		t.Fatal("Fingerprint() followed an https -> http redirect")
	}
	_, err := h.FetchWithFingerprint(context.Background(), src, filepath.Join(t.TempDir(), "a.csv"))
	if err == nil || !strings.Contains(err.Error(), "refusing redirect from https to http") {
		t.Fatalf("FetchWithFingerprint() error = %v, want a clear refusal", err)
	}
	if n := len(plain.requests()); n != 0 {
		t.Errorf("the plain-http server got %d request(s)", n)
	}
}

func TestRedirect_CrossHostDropsCredentialHeaders(t *testing.T) {
	other := newRecorder(false, ok)
	defer other.Close()
	// Same port, different host name: 127.0.0.1 -> localhost is a cross-host redirect.
	u, _ := url.Parse(other.URL)
	otherByName := "http://localhost:" + u.Port() + "/a.csv"
	origin := newRecorder(false, redirectTo(otherByName))
	defer origin.Close()

	src := registry.Source{URL: origin.URL + "/a.csv", Headers: map[string]string{
		"Authorization": "Bearer secret", "X-Api-Key": "secret", "Accept": "text/csv",
	}}
	if _, err := handlerFor(nil).FetchWithFingerprint(context.Background(), src, filepath.Join(t.TempDir(), "a.csv")); err != nil {
		t.Fatalf("FetchWithFingerprint() error = %v", err)
	}
	reqs := other.requests()
	if len(reqs) == 0 {
		t.Fatal("the redirect target got no request")
	}
	for _, h := range reqs {
		for _, name := range []string{"Authorization", "X-Api-Key", "Accept"} {
			if h.Get(name) != "" {
				t.Errorf("%s forwarded across hosts: %q", name, h.Get(name))
			}
		}
	}
}

func TestRedirect_SameHostKeepsHeaders(t *testing.T) {
	srv := newRecorder(false, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		ok(w, r)
	})
	defer srv.Close()
	src := registry.Source{URL: srv.URL + "/old", Headers: map[string]string{"X-Api-Key": "k"}}
	if _, err := handlerFor(nil).FetchWithFingerprint(context.Background(), src, filepath.Join(t.TempDir(), "a")); err != nil {
		t.Fatal(err)
	}
	reqs := srv.requests()
	if got := reqs[len(reqs)-1].Get("X-Api-Key"); got != "k" {
		t.Errorf("X-Api-Key after a same-host redirect = %q, want it kept", got)
	}
}

func TestRedirect_LoopStops(t *testing.T) {
	srv := newRecorder(false, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	})
	defer srv.Close()
	_, err := handlerFor(nil).FetchWithFingerprint(context.Background(), registry.Source{URL: srv.URL + "/a"}, filepath.Join(t.TempDir(), "a"))
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Errorf("error = %v", err)
	}
}
