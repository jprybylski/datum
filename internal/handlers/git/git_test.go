package git

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/jprybylski/datum/internal/registry"
)

// testRepo wraps a local, non-bare git repo whose "origin" remote is a local bare repo -
// entirely in-process, no network and no Docker required. defaultCacheDir() (which datum's
// ensureRepo uses to cache clones) is redirected into a temp dir via XDG_CACHE_HOME for the
// life of the test.
type testRepo struct {
	remoteURL string // path to the bare "remote" repo, usable directly as source.URL
	repo      *gogit.Repository
	workDir   string
	branch    string // the initial branch name go-git created (e.g. "master")
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	base := t.TempDir()
	bareDir := filepath.Join(base, "remote.git")
	if _, err := gogit.PlainInit(bareDir, true); err != nil {
		t.Fatalf("init bare repo: %v", err)
	}

	workDir := filepath.Join(base, "work")
	repo, err := gogit.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init work repo: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{bareDir}}); err != nil {
		t.Fatalf("create remote: %v", err)
	}

	tr := &testRepo{remoteURL: bareDir, repo: repo, workDir: workDir}
	tr.commitFile(t, "hello.txt", "hello world")

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	tr.branch = head.Name().Short()
	tr.push(t, "refs/heads/*:refs/heads/*")
	return tr
}

func (tr *testRepo) commitFile(t *testing.T, relPath, content string) {
	t.Helper()
	full := filepath.Join(tr.workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	wt, err := tr.repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add(relPath); err != nil {
		t.Fatalf("add: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}
	if _, err := wt.Commit("commit "+relPath, &gogit.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func (tr *testRepo) push(t *testing.T, refSpec config.RefSpec) {
	t.Helper()
	err := tr.repo.Push(&gogit.PushOptions{RemoteName: "origin", RefSpecs: []config.RefSpec{refSpec}, Force: true})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		t.Fatalf("push %s: %v", refSpec, err)
	}
}

func (tr *testRepo) tag(t *testing.T, name string) {
	t.Helper()
	head, err := tr.repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}
	if _, err := tr.repo.CreateTag(name, head.Hash(), &gogit.CreateTagOptions{Tagger: sig, Message: name}); err != nil {
		t.Fatalf("create tag %s: %v", name, err)
	}
	tr.push(t, "refs/tags/*:refs/tags/*")
}

func TestNameAndRegistration(t *testing.T) {
	h := New()
	if h.Name() != "git" {
		t.Errorf("Name() = %q, want %q", h.Name(), "git")
	}
	if _, ok := registry.Get("git"); !ok {
		t.Error("git handler was not registered via init()")
	}
}

func TestParseGitSource(t *testing.T) {
	tests := []struct {
		name    string
		src     registry.Source
		wantErr bool
	}{
		{"missing url", registry.Source{Ref: "main", Path: "f.txt"}, true},
		{"missing ref", registry.Source{URL: "u", Path: "f.txt"}, true},
		{"missing path", registry.Source{URL: "u", Ref: "main"}, true},
		{"valid", registry.Source{URL: "u", Ref: "main", Path: "f.txt"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseGitSource(tt.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitSource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("path is slash-normalized", func(t *testing.T) {
		_, _, path, err := parseGitSource(registry.Source{URL: "u", Ref: "main", Path: filepath.Join("a", "b.txt")})
		if err != nil {
			t.Fatalf("parseGitSource() error = %v", err)
		}
		if path != "a/b.txt" {
			t.Errorf("path = %q, want %q", path, "a/b.txt")
		}
	})
}

func TestFingerprintAndFetch_Branch(t *testing.T) {
	tr := newTestRepo(t)
	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: tr.branch, Path: "hello.txt"}

	fp1, err := h.Fingerprint(context.Background(), src)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	if fp1 == "" || fp1[:8] != "gitblob:" {
		t.Errorf("Fingerprint() = %q, want gitblob: prefix", fp1)
	}

	dest := filepath.Join(t.TempDir(), "out", "hello.txt")
	if err := h.Fetch(context.Background(), src, dest); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("fetched content = %q, want %q", got, "hello world")
	}

	// Update the source and confirm the fingerprint changes and re-fetch picks up new content.
	tr.commitFile(t, "hello.txt", "hello again")
	tr.push(t, "refs/heads/*:refs/heads/*")

	fp2, err := h.Fingerprint(context.Background(), src)
	if err != nil {
		t.Fatalf("Fingerprint() after update error = %v", err)
	}
	if fp2 == fp1 {
		t.Error("Fingerprint() did not change after source content changed")
	}

	if err := h.Fetch(context.Background(), src, dest); err != nil {
		t.Fatalf("Fetch() after update error = %v", err)
	}
	got, err = os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read re-fetched file: %v", err)
	}
	if string(got) != "hello again" {
		t.Errorf("re-fetched content = %q, want %q", got, "hello again")
	}
}

func TestFingerprintAndFetch_Tag(t *testing.T) {
	tr := newTestRepo(t)
	tr.tag(t, "v1.0.0")

	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: "v1.0.0", Path: "hello.txt"}

	fp, err := h.Fingerprint(context.Background(), src)
	if err != nil {
		t.Fatalf("Fingerprint() by tag error = %v", err)
	}
	if fp == "" {
		t.Error("Fingerprint() by tag returned empty string")
	}

	dest := filepath.Join(t.TempDir(), "hello.txt")
	if err := h.Fetch(context.Background(), src, dest); err != nil {
		t.Fatalf("Fetch() by tag error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("fetched content = %q, want %q", got, "hello world")
	}
}

func TestFingerprintAndFetch_FullyQualifiedRef(t *testing.T) {
	tr := newTestRepo(t)
	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: "refs/heads/" + tr.branch, Path: "hello.txt"}

	if _, err := h.Fingerprint(context.Background(), src); err != nil {
		t.Fatalf("Fingerprint() with fully-qualified ref error = %v", err)
	}
}

func TestFingerprint_UnknownRef(t *testing.T) {
	tr := newTestRepo(t)
	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: "does-not-exist", Path: "hello.txt"}

	if _, err := h.Fingerprint(context.Background(), src); err == nil {
		t.Error("Fingerprint() with unknown ref expected error, got nil")
	}
}

func TestFingerprint_UnknownPath(t *testing.T) {
	tr := newTestRepo(t)
	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: tr.branch, Path: "does-not-exist.txt"}

	_, err := h.Fingerprint(context.Background(), src)
	if err == nil {
		t.Fatal("Fingerprint() with unknown path expected error, got nil")
	}
}

func TestFingerprint_MissingFields(t *testing.T) {
	h := New()
	if _, err := h.Fingerprint(context.Background(), registry.Source{}); err == nil {
		t.Error("Fingerprint() with empty source expected error, got nil")
	}
	if err := h.Fetch(context.Background(), registry.Source{}, "dest"); err == nil {
		t.Error("Fetch() with empty source expected error, got nil")
	}
}

func TestEnsureRepo_ReusesCache(t *testing.T) {
	tr := newTestRepo(t)

	repo1, err := ensureRepo(tr.remoteURL)
	if err != nil {
		t.Fatalf("ensureRepo() first call error = %v", err)
	}
	repo2, err := ensureRepo(tr.remoteURL)
	if err != nil {
		t.Fatalf("ensureRepo() second call error = %v", err)
	}
	// Both should resolve the same head commit from the same on-disk cache.
	refName := plumbing.ReferenceName("refs/remotes/origin/" + tr.branch)
	h1, err := repo1.Reference(refName, true)
	if err != nil {
		t.Fatalf("resolve ref on repo1: %v", err)
	}
	h2, err := repo2.Reference(refName, true)
	if err != nil {
		t.Fatalf("resolve ref on repo2: %v", err)
	}
	if h1.Hash() != h2.Hash() {
		t.Errorf("cached repo hash mismatch: %v != %v", h1.Hash(), h2.Hash())
	}
}

func TestShortHash(t *testing.T) {
	h1 := shortHash("https://example.com/repo.git")
	h2 := shortHash("https://example.com/repo.git")
	h3 := shortHash("https://example.com/other.git")
	if h1 != h2 {
		t.Error("shortHash() not deterministic")
	}
	if h1 == h3 {
		t.Error("shortHash() collided for different inputs")
	}
	if len(h1) != 16 {
		t.Errorf("shortHash() length = %d, want 16", len(h1))
	}
}

func TestIsUpToDate(t *testing.T) {
	if !isUpToDate(nil) {
		t.Error("isUpToDate(nil) = false, want true")
	}
	if !isUpToDate(gogit.NoErrAlreadyUpToDate) {
		t.Error("isUpToDate(NoErrAlreadyUpToDate) = false, want true")
	}
	if isUpToDate(errors.New("boom")) {
		t.Error("isUpToDate(other error) = true, want false")
	}
}

func TestDefaultCacheDir(t *testing.T) {
	t.Run("uses XDG_CACHE_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/custom/cache")
		if got := defaultCacheDir(); got != "/custom/cache" {
			t.Errorf("defaultCacheDir() = %q, want %q", got, "/custom/cache")
		}
	})

	t.Run("falls back to ~/.cache/datum", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home dir available: %v", err)
		}
		want := filepath.Join(home, ".cache", "datum")
		if got := defaultCacheDir(); got != want {
			t.Errorf("defaultCacheDir() = %q, want %q", got, want)
		}
	})
}

func TestGitAuth_HostKeyOptIn(t *testing.T) {
	t.Run("insecure opt-out is nil by default", func(t *testing.T) {
		t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "")
		if cb := insecureHostKeyCallback(); cb != nil {
			t.Error("insecureHostKeyCallback() should be nil unless explicitly enabled")
		}
	})

	t.Run("insecure opt-out returns a callback when set", func(t *testing.T) {
		t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "1")
		if cb := insecureHostKeyCallback(); cb == nil {
			t.Error("insecureHostKeyCallback() should be non-nil when DATUM_GIT_INSECURE_HOST_KEY=1")
		}
	})

	t.Run("gitAuth does not panic for http/https/ssh URLs", func(t *testing.T) {
		for _, u := range []string{
			"https://example.com/repo.git",
			"http://example.com/repo.git",
			"git@example.com:owner/repo.git",
			"ssh://git@example.com/owner/repo.git",
			"/local/path/without/scheme",
		} {
			_ = gitAuth(u)
		}
	})

	t.Run("https auth picks up GIT_TOKEN", func(t *testing.T) {
		t.Setenv("GIT_TOKEN", "sometoken")
		auth := gitAuth("https://example.com/repo.git")
		if auth == nil {
			t.Error("gitAuth() with GIT_TOKEN set should return BasicAuth, got nil")
		}
	})
}

func TestBlobForPathAtCommit_ReaderContent(t *testing.T) {
	tr := newTestRepo(t)
	repo, err := ensureRepo(tr.remoteURL)
	if err != nil {
		t.Fatalf("ensureRepo() error = %v", err)
	}
	if err := fetchAllRefs(tr.remoteURL, repo); err != nil && !isUpToDate(err) {
		t.Fatalf("fetchAllRefs() error = %v", err)
	}
	commit, err := resolveRefCommit(repo, tr.branch)
	if err != nil {
		t.Fatalf("resolveRefCommit() error = %v", err)
	}
	sha, r, err := blobForPathAtCommit(repo, commit, "hello.txt")
	if err != nil {
		t.Fatalf("blobForPathAtCommit() error = %v", err)
	}
	defer r.Close()
	if sha == "" {
		t.Error("blobForPathAtCommit() returned empty sha")
	}
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("blob content = %q, want %q", content, "hello world")
	}

	if _, _, err := blobForPathAtCommit(repo, commit, "nope.txt"); err == nil {
		t.Error("blobForPathAtCommit() with missing path expected error, got nil")
	}
}

func TestFetch_DestDirCreationFails(t *testing.T) {
	tr := newTestRepo(t)
	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: tr.branch, Path: "hello.txt"}

	// Make a regular file stand in where Fetch needs to MkdirAll a directory.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	dest := filepath.Join(blocker, "sub", "hello.txt")

	if err := h.Fetch(context.Background(), src, dest); err == nil {
		t.Error("Fetch() with unwritable dest dir expected error, got nil")
	}
}

func TestEnsureRepo_CorruptCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	repoURL := "https://example.invalid/owner/repo.git"
	cacheDir := filepath.Join(defaultCacheDir(), "git", shortHash(repoURL))
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	// A non-empty, non-git directory: os.Stat sees it exists, so ensureRepo takes the
	// PlainOpen path, which should fail since it's not actually a git repo.
	if err := os.WriteFile(filepath.Join(cacheDir, "garbage.txt"), []byte("not a repo"), 0o644); err != nil {
		t.Fatalf("write garbage file: %v", err)
	}

	if _, err := ensureRepo(repoURL); err == nil {
		t.Error("ensureRepo() with corrupted cache dir expected error, got nil")
	}
}
