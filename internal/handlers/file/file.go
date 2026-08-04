package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jprybylski/datum/internal/core"
	"github.com/jprybylski/datum/internal/registry"
)

type handler struct{}

func New() *handler             { return &handler{} }
func (h *handler) Name() string { return "file" }

// Fingerprint hashes src.Path. If it's a directory, every file under it is hashed and combined
// into one aggregate "dirsha256:" fingerprint (see core.HashDir); otherwise it's a plain
// "sha256:" hash of the single file's contents.
func (h *handler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	if src.Path == "" {
		return "", errors.New("file: missing source.path")
	}
	info, err := os.Stat(src.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		hh, err := core.HashDir(src.Path)
		if err != nil {
			return "", err
		}
		return "dirsha256:" + hh, nil
	}
	hh, err := core.HashFile(src.Path)
	if err != nil {
		return "", err
	}
	return "sha256:" + hh, nil
}

// Fetch copies src.Path to dest, satisfying the base registry.Fetcher interface. It's a thin
// wrapper around FetchDir with no previous-run state and no other dataset's claimed paths, so a
// directory source gets a fresh copy but not the cross-run cleanup or cross-dataset conflict
// checking that come from threading that state through - callers that need those (the engine)
// call FetchDir directly with state it tracks in the lockfile (see registry.DirManifestFetcher).
func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	_, err := h.FetchDir(ctx, src, dest, nil, nil)
	return err
}

// FetchDir implements registry.DirManifestFetcher. If src.Path is a single file, it behaves like
// Fetch and returns a nil manifest. If it's a directory, its entire contents are recreated under
// dest (see fetchDir), and the relative paths written are returned as the manifest.
func (h *handler) FetchDir(ctx context.Context, src registry.Source, dest string, prevManifest []string, claimed map[string]bool) ([]string, error) {
	if src.Path == "" {
		return nil, errors.New("file: missing source.path")
	}
	info, err := os.Stat(src.Path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return fetchDir(src.Path, dest, prevManifest, claimed)
	}
	return nil, fetchFile(src.Path, dest)
}

func fetchFile(srcPath, dest string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// fetchDir recreates every file from srcDir under destDir, preserving relative paths, and returns
// the sorted list of relative paths it wrote. The caller (the engine) persists that as this
// dataset's manifest - in the lockfile, not a sidecar file on disk - and passes it back in as
// prevManifest on the next call, so cleanup only ever removes files this dataset itself wrote and
// no longer writes now. Files in destDir this dataset never wrote (not in prevManifest) are left
// untouched, since destDir isn't assumed to hold only this dataset's contents.
//
// More than one dataset can target the same destDir. prevManifest and the returned manifest are
// always just this one dataset's own paths, so its cleanup never touches another dataset's files.
// claimed lists relative paths already owned there by other datasets (as of their own last
// fetch); if any path this fetch would write appears in claimed, that's a naming conflict between
// two datasets sharing a target, and the fetch fails instead of silently overwriting (or being
// overwritten by) the other dataset's file. Nothing is written when a conflict is detected.
//
// This isn't a whole-tree atomic operation - files are copied and removed one at a time, same as
// the rest of datum's per-file atomicity (tmp file + rename). A crash partway through can leave
// destDir in a partially-updated state; re-running Fetch will finish the job.
func fetchDir(srcDir, destDir string, prevManifest []string, claimed map[string]bool) ([]string, error) {
	rels, err := core.DirManifest(srcDir)
	if err != nil {
		return nil, err
	}

	if conflicts := conflictingPaths(rels, claimed); len(conflicts) > 0 {
		return nil, fmt.Errorf("file: path(s) already synced here by another dataset: %s", strings.Join(conflicts, ", "))
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}

	for _, rel := range rels {
		if err := fetchFile(filepath.Join(srcDir, rel), filepath.Join(destDir, rel)); err != nil {
			return nil, fmt.Errorf("file: copying %q: %w", rel, err)
		}
	}

	newSet := make(map[string]bool, len(rels))
	for _, r := range rels {
		newSet[r] = true
	}
	for _, prev := range prevManifest {
		if !newSet[prev] {
			// best-effort: file was removed upstream
			_ = os.Remove(filepath.Join(destDir, prev))
			removeEmptyParents(destDir, filepath.Dir(prev))
		}
	}

	return rels, nil
}

// conflictingPaths returns, sorted, every path in rels that's also in claimed - relative paths a
// different dataset already wrote to this same destDir. Two datasets writing disjoint relative
// paths into the same destDir is fine; two datasets both writing the same relative path is
// flagged instead of letting one silently clobber the other.
func conflictingPaths(rels []string, claimed map[string]bool) []string {
	var conflicts []string
	for _, r := range rels {
		if claimed[r] {
			conflicts = append(conflicts, r)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

// removeEmptyParents walks upward from filepath.Join(root, dir), removing directories left empty
// by a file deletion, and stops at the first non-empty directory or at root itself (root is never
// removed, since it's the target directory the dataset owns, not a byproduct of it).
func removeEmptyParents(root, dir string) {
	for dir != "." && dir != string(filepath.Separator) {
		full := filepath.Join(root, dir)
		entries, err := os.ReadDir(full)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(full); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func init() {
	registry.Register(New())
}
