package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

// --cache-dir replaces XDG_CACHE_HOME for the clone cache, and nothing is written there (#43).
func TestCacheDirSetting(t *testing.T) {
	tr := newTestRepo(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	cache := t.TempDir()
	defer registry.Configure(registry.Settings{CacheDir: cache})()

	src := registry.Source{URL: tr.remoteURL, Ref: tr.branch, Path: "hello.txt"}
	if _, err := New().Fingerprint(context.Background(), src); err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "git", shortHash(tr.remoteURL))); err != nil {
		t.Errorf("no clone cache under --cache-dir: %v", err)
	}
	if entries, _ := os.ReadDir(xdg); len(entries) != 0 {
		t.Errorf("wrote under XDG_CACHE_HOME: %v", entries)
	}
}

// --git-https-only refuses other URLs before any repository work, and never builds SSH auth.
func TestGitHTTPSOnlySetting(t *testing.T) {
	tr := newTestRepo(t)
	defer registry.Configure(registry.Settings{GitHTTPSOnly: true})()
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "agent.sock"))
	t.Setenv("GIT_SSH_KEY", filepath.Join(t.TempDir(), "id"))

	for _, url := range []string{tr.remoteURL, "ssh://git@example.com/r.git", "git@example.com:r.git", "git://example.com/r.git", "http://example.com/r.git"} {
		src := registry.Source{URL: url, Ref: "main", Path: "a"}
		if _, err := New().Fingerprint(context.Background(), src); err == nil || !strings.Contains(err.Error(), "only https://") {
			t.Errorf("Fingerprint(%s) error = %v, want the https-only refusal", url, err)
		}
	}
	if auth := gitAuth("ssh://git@example.com/r.git"); auth != nil {
		t.Errorf("gitAuth() = %T, want no SSH auth under --git-https-only", auth)
	}
}
