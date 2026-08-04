package core

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Delete removes the local files tracked for each of ids and marks them Deleted in the lockfile,
// so a later Check/Fetch skips them instead of treating the now-missing target as something to
// re-fetch or fail on (see skipIfDeleted). It never modifies cfgPath - only the lockfile records
// the deletion - so `datum undelete` can resume tracking without the config having drifted.
//
// Unless yes is true, it prints what would be deleted and reads a y/n confirmation line from in
// before touching anything; declining aborts with exit 0 and no changes. yes is meant for
// scripts/CI, where there's no one to prompt.
//
// Returns 0 on success (including "nothing to do"), 2 for a config/lock load error or an id that
// isn't a known dataset, or 1 if any dataset's files couldn't be removed.
func Delete(cfgPath, lockPath string, ids []string, yes bool, in io.Reader, out io.Writer) int {
	if len(ids) == 0 {
		fmt.Fprintln(out, "usage: datum delete ID [ID ...]")
		return 2
	}

	cfg, err := readConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(out, "config error: %v\n", err)
		return 2
	}

	lk, err := readLock(lockPath)
	if err != nil {
		fmt.Fprintf(out, "lock error: %v\n", err)
		return 2
	}

	byID := make(map[string]Dataset, len(cfg.Datasets))
	for _, ds := range cfg.Datasets {
		byID[ds.ID] = ds
	}

	type pending struct {
		ds   Dataset
		item *LockItem
	}
	var toDelete []pending
	for _, id := range ids {
		ds, ok := byID[id]
		if !ok {
			fmt.Fprintf(out, "unknown dataset id: %s\n", id)
			return 2
		}
		item := lk.Items[id]
		if item != nil && item.Deleted {
			fmt.Fprintf(out, "%s: already deleted\n", id)
			continue
		}
		toDelete = append(toDelete, pending{ds: ds, item: item})
	}

	if len(toDelete) == 0 {
		fmt.Fprintln(out, "nothing to delete")
		return 0
	}

	fmt.Fprintln(out, "The following will be deleted:")
	for _, p := range toDelete {
		fmt.Fprintf(out, "  %s -> %s\n", p.ds.ID, p.ds.Target)
	}

	if !yes {
		fmt.Fprintf(out, "Delete %d dataset(s) and their tracked local files? [y/N] ", len(toDelete))
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(out, "reading confirmation: %v\n", err)
			return 1
		}
		if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "aborted, nothing deleted")
			return 0
		}
	}

	now := time.Now().UTC()
	exit := 0
	for _, p := range toDelete {
		if _, err := deleteDatasetFiles(p.ds, p.item, cfg.Datasets); err != nil {
			fmt.Fprintf(out, "%s %s: %v\n", colorize(ansiRed, "[ERR ]"), p.ds.ID, err)
			exit = 1
			continue
		}

		item := lk.Items[p.ds.ID]
		if item == nil {
			item = &LockItem{}
			lk.Items[p.ds.ID] = item
		}
		item.Deleted = true
		item.DeletedAt = &now
		fmt.Fprintf(out, "%s %s: deleted\n", colorize(ansiGreen, "[DEL ]"), p.ds.ID)
	}

	lk.Version = 1
	if err := writeLock(lockPath, lk); err != nil {
		fmt.Fprintf(out, "lock write error: %v\n", err)
		if exit == 0 {
			exit = 1
		}
	}
	return exit
}

// Undelete clears the Deleted flag `datum delete` set for each of ids, so the next Check/Fetch
// resumes treating the dataset normally (a "fail"/"log" policy dataset with a missing target will
// report stale/missing; an "update" policy dataset will simply re-fetch). It only touches the
// lockfile and never re-fetches data itself.
//
// Returns 0 if every id was undeleted, 2 for a lock load error or missing ids, or 1 if any id
// wasn't marked deleted (including ids datum has never heard of).
func Undelete(lockPath string, ids []string, out io.Writer) int {
	if len(ids) == 0 {
		fmt.Fprintln(out, "usage: datum undelete ID [ID ...]")
		return 2
	}

	lk, err := readLock(lockPath)
	if err != nil {
		fmt.Fprintf(out, "lock error: %v\n", err)
		return 2
	}

	exit := 0
	for _, id := range ids {
		item := lk.Items[id]
		if item == nil || !item.Deleted {
			fmt.Fprintf(out, "%s: not marked as deleted\n", id)
			exit = 1
			continue
		}
		item.Deleted = false
		item.DeletedAt = nil
		fmt.Fprintf(out, "%s %s: undeleted (run 'datum fetch %s' to restore data)\n", colorize(ansiGreen, "[UNDEL]"), id, id)
	}

	lk.Version = 1
	if err := writeLock(lockPath, lk); err != nil {
		fmt.Fprintf(out, "lock write error: %v\n", err)
		if exit == 0 {
			exit = 1
		}
	}
	return exit
}

// Unlock permanently removes the lockfile entry for each of ids, regardless of whether the
// dataset is still present in cfgPath. Unlike Delete, it never touches local files - it only
// forgets tracking history. That's why it works the same way on entries `datum delete` already
// removed the files for, and on entries orphaned by editing them out of .data.yaml entirely
// (neither Check nor Fetch ever prune those on their own - see
// TestCheckFetch_OrphanedLockEntryPersists).
//
// Unlocking an entry that's still actively tracked in the config resets its pin: the next check
// under a "fail" policy will report "remote changed" from (none), since there's no longer a
// recorded fingerprint to compare against. The confirmation prompt calls that out explicitly
// rather than silently allowing it or refusing it outright.
//
// cfgPath is read best-effort, only to annotate each id's still-tracked/orphaned status in the
// confirmation prompt - a config read failure doesn't block unlocking, since unlock is
// fundamentally a lockfile-only operation.
//
// Returns 0 if every id was unlocked (or there was nothing to do - see Delete's equivalent "not
// found" note), 2 for a lock load/write error or no ids given, or 1 if any id had no lockfile
// entry to remove.
func Unlock(cfgPath, lockPath string, ids []string, yes bool, in io.Reader, out io.Writer) int {
	if len(ids) == 0 {
		fmt.Fprintln(out, "usage: datum unlock ID [ID ...]")
		return 2
	}

	lk, err := readLock(lockPath)
	if err != nil {
		fmt.Fprintf(out, "lock error: %v\n", err)
		return 2
	}

	inConfig := map[string]bool{}
	if cfg, err := readConfig(cfgPath); err == nil {
		for _, ds := range cfg.Datasets {
			inConfig[ds.ID] = true
		}
	}

	type pending struct {
		id   string
		item *LockItem
	}
	var toUnlock []pending
	exit := 0
	for _, id := range ids {
		item, ok := lk.Items[id]
		if !ok {
			fmt.Fprintf(out, "%s: no lockfile entry\n", id)
			exit = 1
			continue
		}
		toUnlock = append(toUnlock, pending{id: id, item: item})
	}

	if len(toUnlock) == 0 {
		fmt.Fprintln(out, "nothing to unlock")
		return exit
	}

	fmt.Fprintln(out, "The following lockfile entries will be permanently removed:")
	for _, p := range toUnlock {
		fmt.Fprintf(out, "  %s (%s)\n", p.id, unlockNote(p.item, inConfig[p.id]))
	}

	if !yes {
		noun := "entry"
		if len(toUnlock) != 1 {
			noun = "entries"
		}
		fmt.Fprintf(out, "Unlock %d %s? This cannot be undone. [y/N] ", len(toUnlock), noun)
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(out, "reading confirmation: %v\n", err)
			return 1
		}
		if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "aborted, nothing unlocked")
			return 0
		}
	}

	for _, p := range toUnlock {
		delete(lk.Items, p.id)
		fmt.Fprintf(out, "%s %s: unlocked\n", colorize(ansiGreen, "[UNLK ]"), p.id)
	}

	lk.Version = 1
	if err := writeLock(lockPath, lk); err != nil {
		fmt.Fprintf(out, "lock write error: %v\n", err)
		if exit == 0 {
			exit = 1
		}
	}
	return exit
}

// unlockNote describes, for the confirmation listing, why an entry is unlocked: whether it's
// still tracked in the config (unlocking resets its pin), was explicitly deleted, or is orphaned
// (no longer in .data.yaml at all). Deleted and orphaned aren't mutually exclusive - an entry can
// be both - so this covers all four combinations.
func unlockNote(item *LockItem, inConfig bool) string {
	switch {
	case item.Deleted && inConfig:
		return "deleted, still in .data.yaml"
	case item.Deleted:
		return "deleted, orphaned - not in .data.yaml"
	case inConfig:
		return "still tracked in .data.yaml - this resets its pin"
	default:
		return "orphaned - not in .data.yaml"
	}
}

// deleteDatasetFiles removes the local files ds.Target holds for a single dataset, using item's
// recorded state to tell a directory-synced target (registry.DirManifestFetcher; item.DirPaths
// non-empty) from a plain file/whole-directory target. Returns the number of top-level paths
// removed.
//
// For a directory-synced target, only the relative paths this dataset itself wrote (item.DirPaths)
// are removed, then any parent directories left empty by that are cleaned up (removeEmptyParents),
// mirroring the same per-dataset-only cleanup fetchDir does when a file disappears upstream - other
// datasets sharing ds.Target keep their files untouched. ds.Target itself is only removed if no
// other configured dataset also targets it (targetSharedByOtherDataset) and it's now empty.
//
// Otherwise (no recorded DirPaths - a plain file, or a directory target that was never fetched)
// ds.Target is removed outright with os.RemoveAll.
func deleteDatasetFiles(ds Dataset, item *LockItem, allDatasets []Dataset) (int, error) {
	if item != nil && len(item.DirPaths) > 0 {
		removed := 0
		for _, rel := range item.DirPaths {
			full := filepath.Join(ds.Target, rel)
			if err := os.Remove(full); err == nil {
				removed++
			} else if !os.IsNotExist(err) {
				return removed, fmt.Errorf("removing %s: %w", full, err)
			}
			removeEmptyParents(ds.Target, filepath.Dir(rel))
		}
		if !targetSharedByOtherDataset(ds, allDatasets) {
			_ = os.Remove(ds.Target) // best-effort: only succeeds once ds.Target is empty
		}
		return removed, nil
	}

	if !fileExists(ds.Target) {
		return 0, nil
	}
	if err := os.RemoveAll(ds.Target); err != nil {
		return 0, err
	}
	return 1, nil
}

// targetSharedByOtherDataset reports whether some dataset in allDatasets other than ds targets the
// same local path (compared after filepath.Clean, so e.g. "data" and "./data" match) - mirroring
// how claimedPaths compares targets for the fetch side. deleteDatasetFiles uses it to decide
// whether ds.Target itself is safe to remove once ds's own files are gone.
func targetSharedByOtherDataset(ds Dataset, allDatasets []Dataset) bool {
	cleanTarget := filepath.Clean(ds.Target)
	for _, other := range allDatasets {
		if other.ID == ds.ID {
			continue
		}
		if filepath.Clean(other.Target) == cleanTarget {
			return true
		}
	}
	return false
}

// removeEmptyParents walks upward from filepath.Join(root, dir), removing directories left empty
// by a preceding file deletion, and stops at the first non-empty directory or at root itself (root
// is never removed here - deleteDatasetFiles decides separately whether to remove it).
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
