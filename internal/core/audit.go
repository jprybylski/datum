package core

import (
	"encoding/json"
	"fmt"
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
}

// AuditReport is the single top-level JSON document `datum audit --json` prints to stdout.
type AuditReport struct {
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
	cfg, err := readConfig(cfgPath)
	if err != nil {
		return reportError("config error", err)
	}
	lk, err := readLock(lockPath)
	if err != nil {
		return reportError("lock error", err)
	}

	var entries []AuditEntry
	seen := make(map[string]bool, len(cfg.Datasets))
	for _, ds := range cfg.Datasets {
		seen[ds.ID] = true
		policy := firstNonEmpty(ds.Policy, cfg.Defaults.Policy)
		entries = append(entries, buildAuditEntry(ds.ID, policy, ds.Target, true, lk.Items[ds.ID]))
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
	data, err := json.MarshalIndent(AuditReport{Entries: entries}, "", "  ")
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
		fmt.Println()
		if e.Note != "" {
			fmt.Printf("       %s\n", e.Note)
		}
	}

	fmt.Printf("%d dataset(s): %d ok, %d pending, %d deleted, %d orphaned\n",
		len(entries), counts[AuditOK], counts[AuditPending], counts[AuditDeleted], counts[AuditOrphaned])
}
