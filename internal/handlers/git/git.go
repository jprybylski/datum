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

func parseGitSource(src registry.Source) (repoURL string, ref plumbing.ReferenceName, path string, err error) {
	if src.URL == "" || src.Path == "" || src.Ref == "" {
		return "", "", "", errors.New("git: require source.url, source.ref, source.path")
	}
	repoURL = src.URL
	if strings.HasPrefix(src.Ref, "refs/") {
		ref = plumbing.ReferenceName(src.Ref)
	} else {
		// Try branch first; resolveRefCommit will fall back to tag.
		ref = plumbing.NewBranchReferenceName(src.Ref)
	}
	path = filepath.ToSlash(src.Path)
	return repoURL, ref, path, nil
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

func resolveRefCommit(repo *git.Repository, name plumbing.ReferenceName) (*object.Commit, error) {
	ref, err := repo.Reference(name, true)
	if err != nil {
		// If it's a branch ref, try the remote tracking ref
		if strings.HasPrefix(string(name), "refs/heads/") {
			remoteName := strings.Replace(string(name), "refs/heads/", "refs/remotes/origin/", 1)
			ref, err = repo.Reference(plumbing.ReferenceName(remoteName), true)
		}
		// If not a branch or remote didn't work, try tag
		if err != nil && !strings.HasPrefix(string(name), "refs/") {
			if ref2, err2 := repo.Reference(plumbing.NewTagReferenceName(name.String()), true); err2 == nil {
				ref = ref2
				err = nil
			}
		}
		// Still no luck?
		if err != nil {
			return nil, fmt.Errorf("git: cannot resolve ref %q", name)
		}
	}
	hash := ref.Hash()
	// Peel annotated tags
	if tobj, err := repo.TagObject(hash); err == nil {
		hash = tobj.Target
	}
	return repo.CommitObject(hash)
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
	rd, err := f.Blob.Reader()
	if err != nil {
		return "", nil, err
	}
	return f.Blob.Hash.String(), rd, nil
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

	if cb, err := gitssh.NewSSHAgentAuth(user); err == nil {
		cb.HostKeyCallback = xssh.InsecureIgnoreHostKey()
		return cb
	}

	if key := os.Getenv("GIT_SSH_KEY"); key != "" {
		passphrase := os.Getenv("GIT_SSH_PASSPHRASE")
		if pk, err := gitssh.NewPublicKeysFromFile(user, key, passphrase); err == nil {
			pk.HostKeyCallback = xssh.InsecureIgnoreHostKey()
			return pk
		}
	}
	return nil
}

func init() { registry.Register(New()) }
