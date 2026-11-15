package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// AuditStatus is the state `datum audit` reports for a single dataset id - the union of what
// .data.yaml declares and what the lockfile actually has recorded for it.
type AuditStatus string

const (
	AuditOK       AuditStatus = "ok"       // tracked in config, has a lock entry, not deleted
	AuditPending  AuditStatus = "pending"  // tracked in config, never fetched (no lock entry yet)
	AuditDeleted  AuditStatus = "deleted"  // `datum delete` removed it; `datum undelete` clears this
	AuditOrphaned AuditStatus = "orphaned" // has a lock entry but no longer appears in .data.yaml
)

// AuditEntry is one dataset id's combined config+lockfile state, as reported by `datum audit`.
type AuditEntry struct {
	ID                string      `json:"id"`
	Status            AuditStatus `json:"status"`
	InConfig          bool        `json:"in_config"`
	Policy            string      `json:"policy,omitempty"`
	Target            string      `json:"target,omitempty"`
	RemoteFingerprint string      `json:"remote_fingerprint,omitempty"`
	LocalSHA256       string      `json:"local_sha256,omitempty"`
	CheckedAt         *time.Time  `json:"checked_at,omitempty"`
	DeletedAt         *time.Time  `json:"deleted_at,omitempty"`
	InaccessibleAt    *time.Time  `json:"inaccessible_at,omitempty"`
	InaccessibleError string      `json:"inaccessible_error,omitempty"`
	Note              string      `json:"note,omitempty"`

	// Drift is set by --verify-local for tracked datasets: "ok" when the local target still
	// matches the lockfile, "changed", "missing", or "unknown" when the lockfile has no hash
	// to compare with (#41).
	Drift string `json:"drift,omitempty"`
	// Files is set by --verify-local for directory targets whose lockfile records per-file
	// hashes: each recorded file and whether it still matches.
	Files []AuditFile `json:"files,omitempty"`
}

// Drift values reported by audit --verify-local.
const (
	DriftOK      = "ok"
	DriftChanged = "changed"
	DriftMissing = "missing"
	DriftUnknown = "unknown"
)

// AuditFile is one recorded file of a directory target, as reported by audit --verify-local.
type AuditFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Drift  string `json:"drift"`
}

// AuditReport is the single top-level JSON document `datum audit --json` prints to stdout.
type AuditReport struct {
	Format  string       `json:"format"`
	Entries []AuditEntry `json:"entries"`
}

// Audit reports every dataset id known either to cfgPath or lockPath - what .data.yaml declares,
// what the lockfile has recorded for it, and how the two relate (ok/pending/deleted/orphaned) -
// without performing any network or filesystem I/O against the actual data sources. It's a purely
// local, read-only report over the two files Check/Fetch already maintain, meant to surface the
// orphaned/deleted entries `datum unlock` and `datum undelete` act on before you act on them.
//
// Config datasets are listed first, in .data.yaml order; any lockfile entries whose id no longer
// appears in the config (orphaned - see TestCheckFetch_OrphanedLockEntryPersists, which is what
// guarantees they're still there to report) are appended after, sorted by id for determinism.
//
// Returns 0 always (audit doesn't fail on what it finds - that's what `check` is for), or 2 if
// the config or lockfile can't be loaded.
func Audit(cfgPath, lockPath string) int {
	return AuditWith(cfgPath, lockPath, Options{})
}

// AuditWith is Audit with the options in opts. It never needs a source's ${NAME} variables: a
// reference to an unset variable is shown as written (#42). With opts.VerifyLocal it also hashes
// each tracked local target and reports drift against the lockfile (#41).
func AuditWith(cfgPath, lockPath string, opts Options) int {
	cfg, info, err := loadConfig(cfgPath, lockPath, opts, scopeAllLenient)
	if err != nil {
		return reportError("config error", err)
	}
	lk, err := readLock(lockPath)
	if err != nil {
		return reportError("lock error", err)
	}

	entries := []AuditEntry{}
	seen := make(map[string]bool, len(cfg.Datasets))
	for i, ds := range cfg.Datasets {
		seen[ds.ID] = true
		policy := firstNonEmpty(ds.Policy, cfg.Defaults.Policy)
		shown := ds.Target
		if info.root != "" {
			shown = filepath.ToSlash(info.relTargets[i])
		}
		item := lk.Items[ds.ID]
		entry := buildAuditEntry(ds.ID, policy, shown, true, item)
		if opts.VerifyLocal && item != nil && !item.Deleted {
			if info.unresolved[i] {
				entry.Drift = DriftUnknown
			} else {
				entry.Drift, entry.Files = localDrift(ds.Target, item)
			}
		}
		entries = append(entries, entry)
	}

	var orphanIDs []string
	for id := range lk.Items {
		if !seen[id] {
			orphanIDs = append(orphanIDs, id)
		}
	}
	sort.Strings(orphanIDs)
	for _, id := range orphanIDs {
		entries = append(entries, buildAuditEntry(id, "", "", false, lk.Items[id]))
	}

	for i := range entries {
		entries[i].InaccessibleError = redact(entries[i].InaccessibleError)
		entries[i].Note = redact(entries[i].Note)
	}
	if JSONOutput {
		printAuditReport(entries)
	} else {
		printAuditText(entries)
	}
	return 0
}

// buildAuditEntry classifies a single id's status from its (possibly absent) config presence and
// (possibly nil) lock item. A Deleted lock item always reports AuditDeleted, regardless of
// whether the dataset is still in the config, since that flag is what actually governs whether
// Check/Fetch touch it (see skipIfDeleted) - a still-configured but deleted dataset is not "ok".
func buildAuditEntry(id, policy, target string, inConfig bool, item *LockItem) AuditEntry {
	e := AuditEntry{ID: id, InConfig: inConfig, Policy: policy, Target: target}
	if item != nil {
		e.RemoteFingerprint = item.RemoteFingerprint
		e.LocalSHA256 = item.LocalSHA256
		e.CheckedAt = item.CheckedAt
		e.InaccessibleAt = item.InaccessibleAt
		e.InaccessibleError = item.InaccessibleError
	}

	switch {
	case item != nil && item.Deleted:
		e.Status = AuditDeleted
		e.DeletedAt = item.DeletedAt
		if inConfig {
			e.Note = fmt.Sprintf("still in .data.yaml; run 'datum undelete %s' to resume tracking", id)
		} else {
			e.Note = "not in .data.yaml"
		}
	case !inConfig:
		e.Status = AuditOrphaned
		e.Note = fmt.Sprintf("not in .data.yaml; run 'datum unlock %s' to remove its lockfile entry", id)
	case item == nil:
		e.Status = AuditPending
		e.Note = "never fetched"
	default:
		e.Status = AuditOK
	}
	return e
}

func printAuditReport(entries []AuditEntry) {
	data, err := json.MarshalIndent(AuditReport{Format: ReportFormat, Entries: entries}, "", "  ")
	if err != nil {
		fmt.Printf("json encode error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// auditStatusDisplay maps each status to the fixed-width bracketed tag and ANSI color used in
// printAuditText, mirroring the [OK  ]/[FAIL]/etc. tag convention Check/Fetch use.
var auditStatusDisplay = map[AuditStatus]struct {
	tag   string
	color string
}{
	AuditOK:       {"[OK  ]", ansiGreen},
	AuditPending:  {"[PEND]", ansiCyan},
	AuditDeleted:  {"[DEL ]", ansiYellow},
	AuditOrphaned: {"[ORPH]", ansiBlue},
}

// printAuditText prints one line per entry (tag, id, policy/target, and the most relevant
// timestamp), an indented note line when there's something the user should act on, and a summary
// count at the end.
func printAuditText(entries []AuditEntry) {
	if len(entries) == 0 {
		fmt.Println("no datasets tracked")
		return
	}

	counts := map[AuditStatus]int{}
	for _, e := range entries {
		counts[e.Status]++

		display := auditStatusDisplay[e.Status]
		fmt.Printf("%s %s", colorize(display.color, display.tag), e.ID)
		if e.Policy != "" {
			fmt.Printf(": %s policy", e.Policy)
		}
		if e.Target != "" {
			fmt.Printf(", target=%s", e.Target)
		}
		switch {
		case e.Status == AuditDeleted && e.DeletedAt != nil:
			fmt.Printf(", deleted %s", e.DeletedAt.Format(time.RFC3339))
		case e.CheckedAt != nil:
			fmt.Printf(", checked %s", e.CheckedAt.Format(time.RFC3339))
		}
		if e.Drift != "" && e.Drift != DriftOK {
			fmt.Printf(", local %s", e.Drift)
		}
		fmt.Println()
		for _, f := range e.Files {
			if f.Drift != DriftOK {
				fmt.Printf("       %s: %s\n", f.Path, f.Drift)
			}
		}
		if e.Note != "" {
			fmt.Printf("       %s\n", e.Note)
		}
	}

	fmt.Printf("%d dataset(s): %d ok, %d pending, %d deleted, %d orphaned\n",
		len(entries), counts[AuditOK], counts[AuditPending], counts[AuditDeleted], counts[AuditOrphaned])
}

// localDrift compares a tracked target with what the lockfile recorded for it.
func localDrift(target string, item *LockItem) (string, []AuditFile) {
	if !fileExists(target) {
		return DriftMissing, nil
	}
	if len(item.DirFiles) > 0 {
		drift := DriftOK
		files := make([]AuditFile, 0, len(item.DirFiles))
		for _, f := range item.DirFiles {
			entry := AuditFile{Path: f.Path, SHA256: f.SHA256, Size: f.Size, Drift: DriftOK}
			full := filepath.Join(target, filepath.FromSlash(f.Path))
			if !fileExists(full) {
				entry.Drift = DriftMissing
			} else if h, err := HashFile(full); err != nil || h != f.SHA256 {
				entry.Drift = DriftChanged
			}
			if entry.Drift != DriftOK {
				drift = DriftChanged
			}
			files = append(files, entry)
		}
		return drift, files
	}
	if item.LocalSHA256 == "" {
		return DriftUnknown, nil
	}
	if h, err := HashPath(target); err != nil || h != item.LocalSHA256 {
		return DriftChanged, nil
	}
	return DriftOK, nil
}
