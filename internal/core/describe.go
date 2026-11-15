package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// targetDesc is what datum records about a target's bytes after a fetch (#41).
type targetDesc struct {
	sha256 string
	size   int64 // single-file targets only
	dir    bool
	files  []FileEntry
}

// describeTarget hashes a freshly fetched target. For a directory, files lists every file the
// fetch wrote (the manifest), or every file under it when the handler reported no manifest.
// On error the description is partial, but still usable to build a lock item, as before #41.
func describeTarget(target string, manifest []string) (targetDesc, error) {
	info, err := os.Stat(target)
	if err != nil {
		return targetDesc{}, err
	}
	if !info.IsDir() {
		h, err := HashFile(target)
		if err != nil {
			return targetDesc{}, err
		}
		return targetDesc{sha256: h, size: info.Size()}, nil
	}

	d := targetDesc{dir: true}
	rels := manifest
	if rels == nil {
		if rels, err = DirManifest(target); err != nil {
			return d, err
		}
	}
	if d.files, err = describeFiles(target, rels); err != nil {
		return d, err
	}
	d.sha256, err = HashDir(target)
	return d, err
}

func describeFiles(dir string, rels []string) ([]FileEntry, error) {
	files := make([]FileEntry, 0, len(rels))
	for _, rel := range rels {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			return files, err
		}
		h, err := HashFile(full)
		if err != nil {
			return files, err
		}
		files = append(files, FileEntry{Path: rel, SHA256: h, Size: info.Size()})
	}
	return files, nil
}

// lockItem builds the lock entry for a successful fetch. A failed fetch's inaccessible fields
// are cleared because the fetch succeeded.
func (d targetDesc) lockItem(fp string, manifest []string, now time.Time) *LockItem {
	item := &LockItem{LocalSHA256: d.sha256, RemoteFingerprint: fp, DirPaths: manifest, CheckedAt: &now}
	if d.dir {
		if manifest != nil {
			item.DirFiles = d.files
		}
	} else {
		item.Size = d.size
	}
	return item
}

// overlay corrects a staged directory's hash to the one an in-place fetch would record: the
// real target's current tree, minus the files datasets fetched so far no longer write, plus
// everything staged into that folder so far (this dataset's files and those of earlier datasets
// sharing it). That keeps the staged lockfile identical to an in-place fetch's, including when
// other datasets or files share the target folder.
func (d *targetDesc) overlay(staged *stagedTarget, stagedPath string, prev, manifest []string) error {
	if !d.dir {
		return nil
	}
	removed := staged.plan.removeAndList(staged.local, removedPaths(prev, manifest))
	hashes := map[string]string{}
	add := func(dir string, skip map[string]bool) error {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return nil
		}
		rels, err := DirManifest(dir)
		if err != nil {
			return err
		}
		for _, rel := range rels {
			if skip[rel] {
				continue
			}
			h, err := HashFile(filepath.Join(dir, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			hashes[rel] = h
		}
		return nil
	}
	if err := add(staged.local, removed); err != nil {
		return err
	}
	if err := add(stagedPath, nil); err != nil {
		return err
	}
	d.sha256 = hashEntries(hashes)
	return nil
}

// stagedResult is the --json description of what a staged fetch wrote.
func (d targetDesc) stagedResult(staged *stagedTarget, stagedPath string, prev, manifest []string) *StagedResult {
	r := &StagedResult{Target: filepath.ToSlash(staged.rel), Path: stagedPath, Kind: "file", SHA256: d.sha256}
	if d.dir {
		r.Kind = "dir"
		r.Files = d.files
		r.Removed = removedPaths(prev, manifest)
	} else {
		r.Size = d.size
	}
	return r
}

// removedPaths lists, sorted, the paths in prev that manifest no longer has.
func removedPaths(prev, manifest []string) []string {
	keep := make(map[string]bool, len(manifest))
	for _, rel := range manifest {
		keep[rel] = true
	}
	var removed []string
	for _, rel := range prev {
		if !keep[rel] {
			removed = append(removed, rel)
		}
	}
	sort.Strings(removed)
	return removed
}

// hashEntries is HashDir's aggregate over a path -> sha256 map.
func hashEntries(hashes map[string]string) string {
	rels := make([]string, 0, len(hashes))
	for rel := range hashes {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	agg := sha256.New()
	for _, rel := range rels {
		agg.Write([]byte(rel))
		agg.Write([]byte{0})
		agg.Write([]byte(hashes[rel]))
		agg.Write([]byte{'\n'})
	}
	return hex.EncodeToString(agg.Sum(nil))
}
