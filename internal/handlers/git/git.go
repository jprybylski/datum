package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gittransport "github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	xssh "golang.org/x/crypto/ssh"

	"github.com/jprybylski/datum/internal/registry"
)

type handler struct{}

func New() *handler             { return &handler{} }
func (h *handler) Name() string { return "git" }

func (h *handler) Fingerprint(_ context.Context, src registry.Source) (string, error) {
	repoURL, refName, filePath, err := parseGitSource(src)
	if err != nil {
		return "", err
	}

	repo, err := ensureRepo(repoURL)
	if err != nil {
		return "", err
	}

	_ = fetchAllRefs(repoURL, repo) // best-effort

	commit, err := resolveRefCommit(repo, refName)
	if err != nil {
		return "", err
	}

	sha, _, err := blobForPathAtCommit(repo, commit, filePath)
	if err != nil {
		return "", err
	}

	return "gitblob:" + sha, nil
}

func (h *handler) Fetch(_ context.Context, src registry.Source, dest string) error {
	repoURL, refName, filePath, err := parseGitSource(src)
	if err != nil {
		return err
	}

	repo, err := ensureRepo(repoURL)
	if err != nil {
		return err
	}

	_ = fetchAllRefs(repoURL, repo)

	commit, err := resolveRefCommit(repo, refName)
	if err != nil {
		return err
	}

	_, r, err := blobForPathAtCommit(repo, commit, filePath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
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

// --- helpers ---

func parseGitSource(src registry.Source) (repoURL, ref, path string, err error) {
	if src.URL == "" || src.Path == "" || src.Ref == "" {
		return "", "", "", errors.New("git: require source.url, source.ref, source.path")
	}
	return src.URL, src.Ref, filepath.ToSlash(src.Path), nil
}

func ensureRepo(repoURL string) (*git.Repository, error) {
	cacheDir := filepath.Join(defaultCacheDir(), "git", shortHash(repoURL))
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return nil, err
		}
		repo, err := git.PlainInit(cacheDir, true /* bare */)
		if err != nil {
			return nil, err
		}
		_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repoURL}})
		if err != nil && !errors.Is(err, git.ErrRemoteExists) {
			return nil, err
		}
		if err := fetchAllRefs(repoURL, repo); err != nil && !isUpToDate(err) {
			return nil, err
		}
		return repo, nil
	}
	return git.PlainOpen(cacheDir)
}

func fetchAllRefs(repoURL string, repo *git.Repository) error {
	auth := gitAuth(repoURL)

	// Fetch heads
	err1 := repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"},
		Depth:      1,
		Tags:       git.NoTags,
		Force:      true,
	})
	if isUpToDate(err1) {
		err1 = nil
	}

	// Fetch tags
	err2 := repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []config.RefSpec{"+refs/tags/*:refs/tags/*"},
		Depth:      1,
		Tags:       git.AllTags,
		Force:      true,
	})
	if isUpToDate(err2) {
		err2 = nil
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// resolveRefCommit resolves a user-supplied ref (branch name, tag name, or a fully-qualified
// "refs/..." name) to a commit.
//
// Branches are cached locally under refs/remotes/origin/* (see fetchAllRefs), not refs/heads/*,
// since the local repo is a plain fetch cache rather than a real clone. So when we don't know
// upfront whether a bare ref names a branch or a tag, we try, in order: a local branch name (in
// case some other tool populated refs/heads/* directly), the origin remote-tracking branch, then
// a tag. A fully-qualified "refs/heads/X" gets the same remote-tracking fallback, since a user
// writing that literally almost certainly means "the X branch". Any other fully-qualified ref
// (e.g. "refs/tags/X") is used as-is.
//
// Note: this must try all applicable candidates regardless of *why* an earlier one failed - an
// earlier version tried to shortcut this by only falling back to a tag lookup when the ref
// "didn't look like" a refs/heads/* path, but by the time the ref reached here it had always
// already been normalized to refs/heads/*, so that fallback could never trigger and tags were
// silently unresolvable.
func resolveRefCommit(repo *git.Repository, ref string) (*object.Commit, error) {
	var candidates []plumbing.ReferenceName
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		branch := strings.TrimPrefix(ref, "refs/heads/")
		candidates = []plumbing.ReferenceName{
			plumbing.ReferenceName(ref),
			plumbing.ReferenceName("refs/remotes/origin/" + branch),
		}
	case strings.HasPrefix(ref, "refs/"):
		candidates = []plumbing.ReferenceName{plumbing.ReferenceName(ref)}
	default:
		candidates = []plumbing.ReferenceName{
			plumbing.NewBranchReferenceName(ref),
			plumbing.ReferenceName("refs/remotes/origin/" + ref),
			plumbing.NewTagReferenceName(ref),
		}
	}

	var lastErr error
	for _, name := range candidates {
		r, err := repo.Reference(name, true)
		if err != nil {
			lastErr = err
			continue
		}
		hash := r.Hash()
		// Peel annotated tags
		if tobj, err := repo.TagObject(hash); err == nil {
			hash = tobj.Target
		}
		return repo.CommitObject(hash)
	}
	return nil, fmt.Errorf("git: cannot resolve ref %q: %w", ref, lastErr)
}

func blobForPathAtCommit(repo *git.Repository, commit *object.Commit, filePath string) (blobSHA string, r io.ReadCloser, err error) {
	t, err := commit.Tree()
	if err != nil {
		return "", nil, err
	}
	f, err := t.File(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("git: file %q not found at %s", filePath, commit.Hash.String())
	}
	rd, err := f.Reader()
	if err != nil {
		return "", nil, err
	}
	return f.Hash.String(), rd, nil
}

func defaultCacheDir() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "datum")
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}

func isUpToDate(err error) bool {
	return err == nil || errors.Is(err, git.NoErrAlreadyUpToDate)
}

// NOTE: return type is from plumbing/transport, not github.com/go-git/go-git/v5.
func gitAuth(raw string) gittransport.AuthMethod {
	u, _ := url.Parse(raw)

	// HTTPS (PAT/basic)
	if u != nil && (u.Scheme == "http" || u.Scheme == "https") {
		user := os.Getenv("GIT_USERNAME")
		pass := os.Getenv("GIT_PASSWORD")
		if t := os.Getenv("GIT_TOKEN"); t != "" {
			user, pass = "x-access-token", t
		}
		if user != "" || pass != "" {
			return &githttp.BasicAuth{Username: user, Password: pass}
		}
		return nil
	}

	// SSH: try agent, then key file
	user := "git"
	if u != nil && u.User != nil && u.User.Username() != "" {
		user = u.User.Username()
	}

	// Leaving HostKeyCallback unset lets go-git apply its own secure default
	// (ssh.NewKnownHostsCallback, which reads SSH_KNOWN_HOSTS or ~/.ssh/known_hosts).
	// Only bypass host-key verification if the user explicitly opts in - doing this
	// unconditionally would silently expose every SSH fetch to MITM attacks.
	insecure := insecureHostKeyCallback()

	if cb, err := gitssh.NewSSHAgentAuth(user); err == nil {
		if insecure != nil {
			cb.HostKeyCallback = insecure
		}
		return cb
	}

	if key := os.Getenv("GIT_SSH_KEY"); key != "" {
		passphrase := os.Getenv("GIT_SSH_PASSPHRASE")
		if pk, err := gitssh.NewPublicKeysFromFile(user, key, passphrase); err == nil {
			if insecure != nil {
				pk.HostKeyCallback = insecure
			}
			return pk
		}
	}
	return nil
}

// insecureHostKeyCallback returns a callback that skips SSH host-key verification, but only
// when explicitly requested via DATUM_GIT_INSECURE_HOST_KEY=1. Returns nil otherwise, so callers
// leave HostKeyCallback unset and get go-git's secure known_hosts-based default.
func insecureHostKeyCallback() xssh.HostKeyCallback {
	if os.Getenv("DATUM_GIT_INSECURE_HOST_KEY") != "1" {
		return nil
	}
	fmt.Fprintln(os.Stderr, "[WARN] git: SSH host key verification disabled via DATUM_GIT_INSECURE_HOST_KEY=1 (vulnerable to MITM)")
	return xssh.InsecureIgnoreHostKey()
}

func init() { registry.Register(New()) }
