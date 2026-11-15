package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jprybylski/datum/internal/registry"
)

type handler struct{ client *http.Client }

type outputFile interface {
	io.Writer
	Close() error
}

var createOutputFile = func(path string) (outputFile, error) { return os.Create(path) }

func New() *handler {
	return &handler{client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: checkRedirect}}
}
func (h *handler) Name() string { return "http" }

func (h *handler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	if src.URL == "" {
		return "", errors.New("http: missing source.url")
	}
	if src.Body != "" {
		return h.fingerprintRequest(ctx, src, http.MethodPost)
	}
	// Try HEAD for ETag/Last-Modified
	req, err := request(ctx, src, http.MethodHead)
	if err != nil {
		return "", err
	}
	resp, err := h.client.Do(req)
	if err == nil && resp.StatusCode < 400 {
		fp, ok := headerFingerprint(resp.Header, true)
		if closeErr := resp.Body.Close(); closeErr != nil {
			return "", closeErr
		}
		if ok {
			return fp, nil
		}
	} else if resp != nil {
		_ = resp.Body.Close()
	}
	// Fallback: GET and hash (may be large)
	return h.fingerprintRequest(ctx, src, http.MethodGet)
}

func (h *handler) fingerprintRequest(ctx context.Context, src registry.Source, method string) (string, error) {
	req, err := request(ctx, src, method)
	if err != nil {
		return "", err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %s %s: %s", method, src.URL, resp.Status)
	}
	if fp, ok := headerFingerprint(resp.Header, false); ok {
		return fp, nil
	}
	hh := sha256.New()
	if _, err := io.Copy(hh, resp.Body); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hh.Sum(nil)), nil
}

func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	_, err := h.FetchWithFingerprint(ctx, src, dest)
	return err
}

// FetchWithFingerprint downloads the response and derives its fingerprint from that same
// response, avoiding the duplicate request Fetch followed by Fingerprint would otherwise make.
func (h *handler) FetchWithFingerprint(ctx context.Context, src registry.Source, dest string) (string, error) {
	if src.URL == "" {
		return "", errors.New("http: missing source.url")
	}
	method := http.MethodGet
	if src.Body != "" {
		method = http.MethodPost
	}
	req, err := request(ctx, src, method)
	if err != nil {
		return "", err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %s %s: %s", method, src.URL, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	tmp := dest + ".tmp"
	f, err := createOutputFile(tmp)
	if err != nil {
		return "", err
	}

	fp, hasHeaderFingerprint := headerFingerprint(resp.Header, false)
	hh := sha256.New()
	writer := io.MultiWriter(f, hh)
	if _, err := io.Copy(writer, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if !hasHeaderFingerprint {
		fp = "sha256:" + hex.EncodeToString(hh.Sum(nil))
	}
	return fp, nil
}

// maxRedirects matches Go's default redirect limit.
const maxRedirects = 10

// sourceHeadersKey carries the names of a source's configured headers on the request context,
// so checkRedirect knows which headers came from the config.
type sourceHeadersKey struct{}

// checkRedirect is the client's redirect policy (#44):
//   - a redirect from https:// to anything else is refused, so nothing (headers, body, a token
//     in the URL) is ever sent in clear text after starting over TLS;
//   - on a redirect to another host, Authorization and every header from the source's headers
//     map are dropped (Go already drops Authorization, Cookie and WWW-Authenticate for a host
//     outside the original domain, but forwards custom headers such as X-API-Key);
//   - at most 10 redirects are followed, as by default.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	first := via[0]
	if strings.EqualFold(first.URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return fmt.Errorf("refusing redirect from https to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	if !strings.EqualFold(req.URL.Host, first.URL.Host) {
		req.Header.Del("Authorization")
		if names, ok := req.Context().Value(sourceHeadersKey{}).([]string); ok {
			for _, name := range names {
				req.Header.Del(name)
			}
		}
	}
	return nil
}

func request(ctx context.Context, src registry.Source, method string) (*http.Request, error) {
	if len(src.Headers) > 0 {
		names := make([]string, 0, len(src.Headers))
		for name := range src.Headers {
			names = append(names, name)
		}
		ctx = context.WithValue(ctx, sourceHeadersKey{}, names)
	}
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(src.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, src.URL, body)
	if err != nil {
		return nil, err
	}
	for name, value := range src.Headers {
		req.Header.Set(name, value)
	}
	return req, nil
}

func headerFingerprint(header http.Header, allowContentLengthOnly bool) (string, bool) {
	etag := strings.TrimSpace(header.Get("ETag"))
	if etag != "" {
		return "etag:" + etag, true
	}
	lm := header.Get("Last-Modified")
	cl := header.Get("Content-Length")
	if lm != "" || allowContentLengthOnly && cl != "" {
		return fmt.Sprintf("lm:%s|len:%s", lm, cl), true
	}
	return "", false
}

func init() {
	registry.Register(New())
}
