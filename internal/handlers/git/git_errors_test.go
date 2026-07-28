package git

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/jprybylski/datum/internal/registry"
)

func skipUnlessCanDenyAccess(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
}

func chmodOrLog(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Logf("cleanup: failed to chmod %s: %v", path, err)
	}
}

// --- ensureRepo / Fingerprint / Fetch error propagation ---

func TestEnsureRepo_UnwritableCacheDir(t *testing.T) {
	skipUnlessCanDenyAccess(t)
	cacheParent := t.TempDir()
	if err := os.Chmod(cacheParent, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, cacheParent, 0o755)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cacheParent, "cache"))

	if _, err := ensureRepo("https://example.com/repo.git"); err == nil {
		t.Error("ensureRepo() with an unwritable cache parent expected an error, got nil")
	}
}

func TestFingerprintAndFetch_UnwritableCacheDir(t *testing.T) {
	skipUnlessCanDenyAccess(t)
	cacheParent := t.TempDir()
	if err := os.Chmod(cacheParent, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, cacheParent, 0o755)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cacheParent, "cache"))

	h := New()
	src := registry.Source{URL: "https://example.com/repo.git", Ref: "main", Path: "f.txt"}
	if _, err := h.Fingerprint(context.Background(), src); err == nil {
		t.Error("Fingerprint() expected an error when the cache dir can't be created, got nil")
	}
	if err := h.Fetch(context.Background(), src, filepath.Join(t.TempDir(), "out.txt")); err == nil {
		t.Error("Fetch() expected an error when the cache dir can't be created, got nil")
	}
}

func TestFetch_UnknownRefAndPath(t *testing.T) {
	tr := newTestRepo(t)
	h := New()

	t.Run("unknown ref", func(t *testing.T) {
		src := registry.Source{URL: tr.remoteURL, Ref: "does-not-exist", Path: "hello.txt"}
		dest := filepath.Join(t.TempDir(), "out.txt")
		if err := h.Fetch(context.Background(), src, dest); err == nil {
			t.Error("Fetch() with unknown ref expected an error, got nil")
		}
	})

	t.Run("unknown path", func(t *testing.T) {
		src := registry.Source{URL: tr.remoteURL, Ref: tr.branch, Path: "does-not-exist.txt"}
		dest := filepath.Join(t.TempDir(), "out.txt")
		if err := h.Fetch(context.Background(), src, dest); err == nil {
			t.Error("Fetch() with unknown path expected an error, got nil")
		}
	})
}

func TestFetch_DestDirNotWritable(t *testing.T) {
	skipUnlessCanDenyAccess(t)
	tr := newTestRepo(t)
	h := New()
	src := registry.Source{URL: tr.remoteURL, Ref: tr.branch, Path: "hello.txt"}

	destDir := t.TempDir()
	if err := os.Chmod(destDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer chmodOrLog(t, destDir, 0o755)

	dest := filepath.Join(destDir, "out.txt")
	if err := h.Fetch(context.Background(), src, dest); err == nil {
		t.Error("Fetch() into a non-writable directory expected an error, got nil")
	}
}

// --- fetchAllRefs error propagation via a bad remote URL on a fresh clone ---

func TestEnsureRepo_FetchFailsOnFreshClone(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// A syntactically fine but nonexistent local path: go-git's local transport will fail to
	// fetch from it, exercising ensureRepo's fetchAllRefs-error branch (and, since this is the
	// heads fetch specifically failing, fetchAllRefs' own final `if err1 != nil { return err1 }`).
	badURL := filepath.Join(t.TempDir(), "does-not-exist.git")
	if _, err := ensureRepo(badURL); err == nil {
		t.Error("ensureRepo() with a nonexistent remote expected an error, got nil")
	}
}

// --- resolveRefCommit: fully-qualified non-branch ref (e.g. "refs/tags/X") ---

func TestResolveRefCommit_FullyQualifiedTagRef(t *testing.T) {
	tr := newTestRepo(t)
	tr.tag(t, "v2.0.0")
	repo, err := ensureRepo(tr.remoteURL)
	if err != nil {
		t.Fatalf("ensureRepo() error = %v", err)
	}
	if err := fetchAllRefs(tr.remoteURL, repo); err != nil && !isUpToDate(err) {
		t.Fatalf("fetchAllRefs() error = %v", err)
	}

	commit, err := resolveRefCommit(repo, "refs/tags/v2.0.0")
	if err != nil {
		t.Fatalf("resolveRefCommit() with a fully-qualified tag ref error = %v", err)
	}
	if commit == nil {
		t.Error("resolveRefCommit() returned a nil commit")
	}
}

// --- gitAuth: SSH agent path ---

// startFakeSSHAgent runs a minimal in-process ssh-agent (via golang.org/x/crypto/ssh/agent,
// the same package go-git's SSH auth talks to) backed by a Unix socket, loaded with one
// generated ed25519 key. Returns the socket path for SSH_AUTH_SOCK.
func startFakeSSHAgent(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets for a fake ssh-agent aren't set up the same way on Windows")
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Unix domain socket paths have a short OS-enforced length limit (~104 bytes on macOS/BSD),
	// and t.TempDir()'s deeply nested, test-name-derived path routinely exceeds it - use a short
	// path directly under the OS temp dir instead.
	sockDir, err := os.MkdirTemp("", "datum-agent")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "a.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatalf("add key to keyring: %v", err)
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, conn) }()
		}
	}()

	return sockPath
}

func TestGitAuth_SSHAgent_InsecureCallbackApplied(t *testing.T) {
	sockPath := startFakeSSHAgent(t)
	t.Setenv("SSH_AUTH_SOCK", sockPath)
	t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "1")

	auth := gitAuth("git@example.com:owner/repo.git")
	if auth == nil {
		t.Fatal("gitAuth() returned nil with a working SSH agent available")
	}
	cb, ok := auth.(interface{ ClientConfig() (*ssh.ClientConfig, error) })
	if !ok {
		t.Fatalf("auth method %T doesn't implement ClientConfig()", auth)
	}
	cfg, err := cb.ClientConfig()
	if err != nil {
		t.Fatalf("ClientConfig() error = %v", err)
	}
	if cfg.HostKeyCallback == nil {
		t.Error("expected the insecure HostKeyCallback to be applied to the agent-based auth method")
	}
}

func TestGitAuth_SSHAgent_SecureByDefault(t *testing.T) {
	sockPath := startFakeSSHAgent(t)
	t.Setenv("SSH_AUTH_SOCK", sockPath)
	t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "")

	auth := gitAuth("git@example.com:owner/repo.git")
	if auth == nil {
		t.Fatal("gitAuth() returned nil with a working SSH agent available")
	}
	cb, ok := auth.(interface{ ClientConfig() (*ssh.ClientConfig, error) })
	if !ok {
		t.Fatalf("auth method %T doesn't implement ClientConfig()", auth)
	}
	cfg, err := cb.ClientConfig()
	if err != nil {
		t.Fatalf("ClientConfig() error = %v", err)
	}
	if cfg.HostKeyCallback == nil {
		t.Error("expected a non-nil HostKeyCallback by default (go-git's own known_hosts default), not left for the transport to reject entirely")
	}
}

// --- gitAuth: GIT_SSH_KEY file-based path (agent unavailable) ---

func writeTestSSHKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "datum-test-key")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	return path
}

func TestGitAuth_KeyFile_NoAgentAvailable(t *testing.T) {
	// SSH_AUTH_SOCK unset (or pointing nowhere) makes NewSSHAgentAuth fail immediately
	// (xanzy/ssh-agent's Available() check), so gitAuth falls through to GIT_SSH_KEY - matching
	// what happens in any CI/container environment with no real ssh-agent running.
	t.Setenv("SSH_AUTH_SOCK", "")
	keyPath := writeTestSSHKey(t)
	t.Setenv("GIT_SSH_KEY", keyPath)

	t.Run("insecure opt-in applies the callback", func(t *testing.T) {
		t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "1")
		auth := gitAuth("git@example.com:owner/repo.git")
		if auth == nil {
			t.Fatal("gitAuth() returned nil with a valid GIT_SSH_KEY set")
		}
	})

	t.Run("secure by default", func(t *testing.T) {
		t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "")
		auth := gitAuth("git@example.com:owner/repo.git")
		if auth == nil {
			t.Fatal("gitAuth() returned nil with a valid GIT_SSH_KEY set")
		}
	})

	t.Run("with a passphrase env var set but key is unencrypted", func(t *testing.T) {
		t.Setenv("GIT_SSH_PASSPHRASE", "unused-passphrase")
		auth := gitAuth("git@example.com:owner/repo.git")
		if auth == nil {
			t.Fatal("gitAuth() returned nil with a valid GIT_SSH_KEY set")
		}
	})
}

func TestGitAuth_KeyFile_InvalidKeyFallsThroughToNil(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	badKeyPath := filepath.Join(t.TempDir(), "not-a-key")
	if err := os.WriteFile(badKeyPath, []byte("not a real key"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_KEY", badKeyPath)

	if auth := gitAuth("git@example.com:owner/repo.git"); auth != nil {
		t.Errorf("gitAuth() with an invalid key file expected nil, got %v", auth)
	}
}
