package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jprybylski/datum/internal/core"
	"github.com/jprybylski/datum/internal/registry"
)

// manifestSuffix names the sidecar file (a sibling of the target directory, not inside it) that
// directory-mode Fetch uses to remember which relative paths it wrote on the previous run. That's
// how it knows which target files to remove when a file disappears from the source, without
// touching anything else in the target directory that this dataset didn't write itself.
const manifestSuffix = ".datum-manifest.json"

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

// Fetch copies src.Path to dest. If src.Path is a directory, its entire contents are recreated
// under dest (see fetchDir); otherwise it's a single-file copy.
func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	if src.Path == "" {
		return errors.New("file: missing source.path")
	}
	info, err := os.Stat(src.Path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fetchDir(src.Path, dest)
	}
	return fetchFile(src.Path, dest)
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

// fetchDir recreates every file from srcDir under destDir, preserving relative paths, and
// removes any file from destDir that this dataset wrote on a previous fetch but that no longer
// exists in srcDir - so files deleted upstream don't linger in the target. Files in destDir that
// this dataset never wrote (tracked via the sidecar manifest) are left untouched, since destDir
// isn't assumed to hold only this dataset's contents.
//
// This isn't a whole-tree atomic operation - files are copied and removed one at a time, same as
// the rest of datum's per-file atomicity (tmp file + rename). A crash partway through can leave
// destDir in a partially-updated state; re-running Fetch will finish the job.
func fetchDir(srcDir, destDir string) error {
	rels, err := core.DirManifest(srcDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for _, rel := range rels {
		if err := fetchFile(filepath.Join(srcDir, rel), filepath.Join(destDir, rel)); err != nil {
			return fmt.Errorf("file: copying %q: %w", rel, err)
		}
	}

	manifestPath := destDir + manifestSuffix
	// A missing or corrupt manifest just means there's no prior state to diff against (e.g.
	// first fetch), not a fetch failure - nothing to delete yet.
	prevRels, _ := readManifest(manifestPath)
	newSet := make(map[string]bool, len(rels))
	for _, r := range rels {
		newSet[r] = true
	}
	for _, prev := range prevRels {
		if !newSet[prev] {
			_ = os.Remove(filepath.Join(destDir, prev)) // best-effort: file was removed upstream
		}
	}

	return writeManifest(manifestPath, rels)
}

func readManifest(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rels []string
	if err := json.Unmarshal(b, &rels); err != nil {
		return nil, err
	}
	return rels, nil
}

func writeManifest(path string, rels []string) error {
	b, err := json.Marshal(rels)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func init() {
	registry.Register(New())
}
