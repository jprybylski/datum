//go:build integration

// Package integration holds tests that need real infrastructure (here: a git-over-SSH server
// in Docker) rather than the fully in-process fixtures the unit test suites use. It's gated
// behind the "integration" build tag so `go test ./...` never picks it up, and is run via
// scripts/test-integration.sh, which brings up docker-compose.yml first.
//
// These tests specifically cover what internal/handlers/git's unit tests (local file:// remotes,
// no real network) can't: the real SSH transport, and in particular that host-key verification
// is genuinely enforced by default and genuinely bypassable via DATUM_GIT_INSECURE_HOST_KEY=1 -
// the security fix this whole harness exists to regression-test.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jprybylski/datum/internal/handlers/git"
	"github.com/jprybylski/datum/internal/registry"
)

const repoURL = "ssh://gituser@localhost:2222/srv/git/repo.git"

// sshTestEnv points the git handler at the ephemeral key scripts/test-integration.sh generated
// for this run, and forces deterministic auth-method selection: with SSH_AUTH_SOCK cleared,
// gitAuth's SSH-agent attempt fails immediately (see xanzy/ssh-agent's Available() check), so it
// falls through to GIT_SSH_KEY exactly as it would in an agent-less CI/container environment.
func sshTestEnv(t *testing.T) {
	t.Helper()
	keyPath := os.Getenv("DATUM_TEST_GIT_SSH_KEY")
	if keyPath == "" {
		t.Skip("DATUM_TEST_GIT_SSH_KEY not set - run via scripts/test-integration.sh, not `go test` directly")
	}
	t.Setenv("GIT_SSH_KEY", keyPath)
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func TestGitHandler_SSH_HostKeyVerifiedByDefault(t *testing.T) {
	sshTestEnv(t)
	t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "")

	h := git.New()
	src := registry.Source{URL: repoURL, Ref: "main", Path: "hello.txt"}

	if _, err := h.Fingerprint(context.Background(), src); err == nil {
		t.Fatal("expected host-key verification to reject an unrecognized host by default, got nil error")
	}
}

func TestGitHandler_SSH_InsecureOptInFetchesOverRealSSH(t *testing.T) {
	sshTestEnv(t)
	t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "1")

	h := git.New()
	src := registry.Source{URL: repoURL, Ref: "main", Path: "hello.txt"}

	fp, err := h.Fingerprint(context.Background(), src)
	if err != nil {
		t.Fatalf("Fingerprint() over real SSH error = %v", err)
	}
	if fp == "" {
		t.Error("Fingerprint() returned an empty string")
	}

	dest := filepath.Join(t.TempDir(), "hello.txt")
	if err := h.Fetch(context.Background(), src, dest); err != nil {
		t.Fatalf("Fetch() over real SSH error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	const want = "hello from the integration test git server\n"
	if string(got) != want {
		t.Errorf("fetched content = %q, want %q", got, want)
	}
}

func TestGitHandler_SSH_Tag(t *testing.T) {
	sshTestEnv(t)
	t.Setenv("DATUM_GIT_INSECURE_HOST_KEY", "1")

	h := git.New()
	src := registry.Source{URL: repoURL, Ref: "v1.0.0", Path: "hello.txt"}

	if _, err := h.Fingerprint(context.Background(), src); err != nil {
		t.Fatalf("Fingerprint() by tag over real SSH error = %v", err)
	}
}
