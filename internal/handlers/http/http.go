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

func New() *handler             { return &handler{client: &http.Client{Timeout: 60 * time.Second}} }
func (h *handler) Name() string { return "http" }

func (h *handler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	if src.URL == "" {
		return "", errors.New("http: missing source.url")
	}
	// Try HEAD for ETag/Last-Modified
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, src.URL, nil)
	resp, err := h.client.Do(req)
	if err == nil && resp.StatusCode < 400 {
		etag := strings.TrimSpace(resp.Header.Get("ETag"))
		if etag != "" {
			resp.Body.Close()
			return "etag:" + etag, nil
		}
		lm := resp.Header.Get("Last-Modified")
		cl := resp.Header.Get("Content-Length")
		resp.Body.Close()
		if lm != "" || cl != "" {
			return fmt.Sprintf("lm:%s|len:%s", lm, cl), nil
		}
	}
	// Fallback: GET and hash (may be large)
	reqG, _ := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	resp2, err := h.client.Do(reqG)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode >= 400 {
		return "", fmt.Errorf("http GET %s: %s", src.URL, resp2.Status)
	}
	hh := sha256.New()
	if _, err := io.Copy(hh, resp2.Body); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hh.Sum(nil)), nil
}

func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	if src.URL == "" {
		return errors.New("http: missing source.url")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http GET %s: %s", src.URL, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

func init() {
	registry.Register(New())
}
