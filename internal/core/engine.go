package core

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jprybylski/datum/internal/registry"
)

// sourceAttempt tries each source in order, calling attempt for each until one succeeds.
//
// This is the shared "try each source, fall back to the next on failure" loop used by both
// Check() and Fetch() for both fingerprint-only and fetch+fingerprint operations. When there's
// more than one source, it prints a [WARN] line for every failed attempt (with an optional
// sub-step label, e.g. "fetch" vs "fingerprint after fetch") before moving to the next source.
// Progress lines are written to w rather than directly to stdout, so callers can buffer each
// dataset's output separately for deterministic ordering under concurrent processing.
//
// Returns the value from the first successful attempt, or the last (unwrapped) error if every
// source failed. Callers are responsible for the final [ERR ] summary line and any error
// wrapping they need for storage (e.g. LockItem.InaccessibleError), since those differ slightly
// between Check() and Fetch().
//
// res is optional (nil when the caller isn't collecting structured --json output); when non-nil,
// every [WARN] line's message is also appended to res.Warnings, so JSON output carries the same
// source-fallback trail as the human-readable text.
func sourceAttempt[T any](w io.Writer, res *Result, dsID string, sources []registry.Source, attempt func(f registry.Fetcher, source registry.Source) (value T, warnLabel string, err error)) (T, error) {
	warn := func(msg string) {
		fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiYellow, "[WARN]"), dsID, msg)
		if res != nil {
			res.Warnings = append(res.Warnings, msg)
		}
	}

	var zero T
	var lastErr error
	for i, source := range sources {
		f, ok := registry.Get(source.Type)
		if !ok {
			lastErr = fmt.Errorf("unknown source.type=%q", source.Type)
			if len(sources) > 1 {
				warn(fmt.Sprintf("source %d/%d: %v (trying next source)", i+1, len(sources), lastErr))
			}
			continue
		}

		value, warnLabel, err := attempt(f, source)
		if err != nil {
			lastErr = err
			if len(sources) > 1 {
				if warnLabel != "" {
					warn(fmt.Sprintf("source %d/%d: %s: %v (trying next source)", i+1, len(sources), warnLabel, err))
				} else {
					warn(fmt.Sprintf("source %d/%d: %v (trying next source)", i+1, len(sources), err))
				}
			}
			continue
		}

		return value, nil
	}
	return zero, lastErr
}

// writeFingerprintChange writes a "remote changed" block as a header line naming the dataset,
// followed by the old (dimmed) and new fingerprint values each on their own indented line.
// Fingerprints are often full sha256 hashes or ETag values 60+ characters long, so cramming both
// onto one "(old -> now)" line reads as a wall of hex; splitting them out is far easier to scan.
// indent should visually line up under dsID, i.e. match the width of "coloredTag " once ANSI
// codes are stripped (7 spaces for the 6-char tags like "[FAIL] ", 8 for 7-char tags like
// "[STALE] ").
func writeFingerprintChange(w io.Writer, coloredTag, indent, dsID, lockfp, fp string) {
	fmt.Fprintf(w, "%s %s: remote changed\n", coloredTag, dsID)
	fmt.Fprintf(w, "%slock: %s\n", indent, colorize(ansiDim, lockfp))
	fmt.Fprintf(w, "%snow:  %s\n", indent, fp)
}

// fingerprintAttempt builds a sourceAttempt callback that just computes a fingerprint.
func fingerprintAttempt(ctx context.Context) func(registry.Fetcher, registry.Source) (string, string, error) {
	return func(f registry.Fetcher, source registry.Source) (string, string, error) {
		fp, err := f.Fingerprint(ctx, source)
		if err != nil {
			return "", "fingerprint", err
		}
		return fp, "", nil
	}
}

// fetchResult is what a successful fetchAttempt produces: the fingerprint to record, plus (for
// handlers that populate a directory tree) the manifest of relative paths written, which the
// caller persists on the dataset's LockItem.DirPaths for next time.
type fetchResult struct {
	fp       string
	manifest []string
}

// fetchAttempt builds a sourceAttempt callback that fetches into dest, then re-fingerprints the
// source so the lockfile records the fingerprint that actually corresponds to what was fetched.
// A source only counts as succeeded if both steps succeed.
//
// If the handler implements registry.DirManifestFetcher, FetchDir is called instead of Fetch,
// threading prevManifest (this dataset's own manifest from its last fetch) and claimed (relative
// paths other datasets targeting the same dest already own, see claimedPaths) through so the
// handler can safely share dest with other datasets and detect naming conflicts between them.
func fetchAttempt(ctx context.Context, dest string, prevManifest []string, claimed map[string]bool) func(registry.Fetcher, registry.Source) (fetchResult, string, error) {
	return func(f registry.Fetcher, source registry.Source) (fetchResult, string, error) {
		var manifest []string
		if mf, ok := f.(registry.DirManifestFetcher); ok {
			m, err := mf.FetchDir(ctx, source, dest, prevManifest, claimed)
			if err != nil {
				return fetchResult{}, "fetch", err
			}
			manifest = m
		} else if err := f.Fetch(ctx, source, dest); err != nil {
			return fetchResult{}, "fetch", err
		}
		fp, err := f.Fingerprint(ctx, source)
		if err != nil {
			return fetchResult{}, "fingerprint after fetch", err
		}
		return fetchResult{fp: fp, manifest: manifest}, "", nil
	}
}

// claimedPaths returns the set of relative paths that, as of the lockfile, belong to some other
// dataset whose Target is the same directory as target (compared after filepath.Clean, so e.g.
// "data" and "./data" match). Multiple datasets are allowed to sync into the same target
// directory; this is how a DirManifestFetcher handler knows which relative paths it must not
// write, because a different dataset already owns them there.
//
// Note this is a best-effort, read-then-fetch check like the rest of datum's fetch pipeline (see
// fetchDir's atomicity note in the file handler) - it isn't safe against two datasets that share a
// target being fetched fully concurrently (--concurrency > 1) and racing past this check at the
// same time. Configs that share a target should be fetched at --concurrency 1 for the guarantee
// to hold.
func claimedPaths(datasets []Dataset, store *lockStore, selfID, target string) map[string]bool {
	claimed := map[string]bool{}
	cleanTarget := filepath.Clean(target)
	for _, other := range datasets {
		if other.ID == selfID || filepath.Clean(other.Target) != cleanTarget {
			continue
		}
		if item := store.get(other.ID); item != nil {
			for _, rel := range item.DirPaths {
				claimed[rel] = true
			}
		}
	}
	return claimed
}

// lockStore provides mutex-guarded access to a Lock's Items map, so multiple datasets can be
// processed concurrently (concurrency > 1) without racing on the shared map. Once a *LockItem is
// obtained via get/ensure, mutating its fields directly is safe without further locking, since
// each dataset ID is only ever touched by the one goroutine processing that dataset.
type lockStore struct {
	mu sync.Mutex
	lk *Lock
}

func (s *lockStore) get(id string) *LockItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lk.Items[id]
}

func (s *lockStore) set(id string, item *LockItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lk.Items[id] = item
}

// ensure returns the existing item for id, or creates, stores, and returns a new empty one.
func (s *lockStore) ensure(id string) *LockItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.lk.Items[id]
	if item == nil {
		item = &LockItem{}
		s.lk.Items[id] = item
	}
	return item
}

// runConcurrently processes n items with at most `concurrency` running at once, calling fn(i)
// for each index. A concurrency of 1 runs items strictly in order, one at a time - this is what
// keeps Check/Fetch's default (sequential) output identical to before concurrency support was
// added, since fn always buffers its own output and the caller prints buffers back in original
// index order regardless of completion order.
func runConcurrently(n, concurrency int, fn func(i int)) {
	var g errgroup.Group
	if concurrency > 0 {
		g.SetLimit(concurrency)
	}
	for i := 0; i < n; i++ {
		i := i
		g.Go(func() error {
			fn(i)
			return nil
		})
	}
	_ = g.Wait() // fn never returns an error; failures are tracked via per-dataset exit codes
}

// skipIfDeleted reports whether ds.ID has been removed by `datum delete` (item.Deleted), and if
// so, writes the standard skip line/result before returning true. Both checkOneDataset and
// fetchOneDataset call this before doing any source or filesystem work, so a deleted dataset never
// gets re-fetched or flagged as changed/missing until `datum undelete` clears the flag.
func skipIfDeleted(w io.Writer, res *Result, dsID string, item *LockItem) bool {
	if item == nil || !item.Deleted {
		return false
	}
	msg := fmt.Sprintf("deleted - skipping (run 'datum undelete %s' to resume tracking)", dsID)
	fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiYellow, "[SKIP]"), dsID, msg)
	if res != nil {
		res.Status = StatusDeleted
		res.Message = msg
	}
	return true
}

// Check verifies all configured datasets against the lockfile according to their policies.
//
// This is the main verification function for datum. It loads the configuration and lockfile,
// then for each dataset:
//  1. Computes the current remote fingerprint
//  2. Compares it against the recorded fingerprint in the lockfile
//  3. Applies the dataset's policy (fail, update, or log)
//  4. Updates the lockfile (only for "update" policy)
//
// Policies explained:
//   - "fail": Exit with error if remote has changed (strict mode for CI/CD) - does not update lockfile
//   - "update": Automatically fetch new data if remote has changed - updates lockfile
//   - "log": Report changes but don't fail or update (monitoring mode) - does not update lockfile
//
// Parameters:
//   - ctx: Controls cancellation/timeout for all handler operations across all datasets
//   - cfgPath: Path to the configuration file (.data.yaml)
//   - lockPath: Path to the lockfile (.data.lock.yaml)
//   - concurrency: Maximum number of datasets to process in parallel (1 = sequential)
//
// Returns:
//   - 0: All datasets are up-to-date (success)
//   - 1: One or more datasets failed verification or had fetch errors
//   - 2: Configuration error or unknown handler type
func Check(ctx context.Context, cfgPath, lockPath string, concurrency int) int {
	// Load configuration file
	cfg, err := readConfig(cfgPath)
	if err != nil {
		return reportError("config error", err)
	}

	// Load lockfile (or create empty one if it doesn't exist)
	lk, err := readLock(lockPath)
	if err != nil {
		return reportError("lock error", err)
	}
	store := &lockStore{lk: lk}

	now := time.Now().UTC()

	// Process each dataset, buffering its output so results print back in the original
	// config-file order regardless of how goroutines actually interleave/complete. In JSON mode
	// the text writer is discarded entirely - results[i] carries the structured outcome instead.
	outputs := make([]string, len(cfg.Datasets))
	results := make([]Result, len(cfg.Datasets))
	exitCodes := make([]int, len(cfg.Datasets))
	runConcurrently(len(cfg.Datasets), concurrency, func(i int) {
		ds := cfg.Datasets[i]
		policy := firstNonEmpty(ds.Policy, cfg.Defaults.Policy)
		results[i].ID = ds.ID
		var out strings.Builder
		w := io.Writer(&out)
		if JSONOutput {
			w = io.Discard
		}
		exitCodes[i] = checkOneDataset(ctx, w, &results[i], ds, policy, store, now, cfg.Datasets)
		outputs[i] = out.String()
	})

	exit := 0
	for i := range exitCodes {
		if exitCodes[i] > exit {
			exit = exitCodes[i]
		}
	}

	// Write updated lockfile back to disk
	lk.Version = 1
	lk.LastChecked = &now
	lockErr := writeLock(lockPath, lk)
	if lockErr != nil && exit == 0 {
		exit = 1
	}

	if JSONOutput {
		printReport(results, lockErr)
	} else {
		for _, out := range outputs {
			fmt.Print(out)
		}
		if lockErr != nil {
			fmt.Printf("lock write error: %v\n", lockErr)
		}
	}
	return exit
}

// checkOneDataset runs Check's per-dataset logic, writing progress lines to w and reading/writing
// this dataset's LockItem through store. Returns this dataset's contribution to the exit code.
//
// res is optional (nil unless the caller is building --json output); when non-nil it's populated
// with the same outcome the text lines describe, so the two output modes never disagree.
func checkOneDataset(ctx context.Context, w io.Writer, res *Result, ds Dataset, policy string, store *lockStore, now time.Time, allDatasets []Dataset) int {
	sources := ds.GetSources()

	// Get the lock entry for this dataset (may be nil if this is the first run)
	item := store.get(ds.ID)
	if skipped := skipIfDeleted(w, res, ds.ID, item); skipped {
		return 0
	}

	// Compute the current remote fingerprint, trying each source in order until one succeeds
	fp, err := sourceAttempt(w, res, ds.ID, sources, fingerprintAttempt(ctx))
	if err != nil {
		var msg string
		if len(sources) > 1 {
			msg = fmt.Sprintf("all %d sources failed, last error: %v", len(sources), err)
		} else {
			msg = fmt.Sprintf("fingerprint: %v", err)
		}
		fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiRed, "[ERR ]"), ds.ID, msg)
		if res != nil {
			res.Status = StatusError
			res.Message = msg
		}
		return 1
	}

	lockfp := "(none)"
	if item != nil {
		lockfp = item.RemoteFingerprint
	}

	// Compute local hash if the target exists (file or directory)
	localHash := ""
	if fileExists(ds.Target) {
		if h, err := HashPath(ds.Target); err == nil {
			localHash = h
		} else {
			msg := fmt.Sprintf("local hash: %v", err)
			fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiRed, "[ERR ]"), ds.ID, msg)
			if res != nil {
				res.Warnings = append(res.Warnings, msg)
			}
		}
	}

	// Determine if the remote source has changed since last check
	stale := (item == nil) || (item.RemoteFingerprint != fp)

	switch policy {
	case "update":
		// UPDATE policy: Automatically fetch if remote changed or local target is missing
		if stale || !fileExists(ds.Target) {
			fmt.Fprintf(w, "%s %s: refreshing\n", colorize(ansiCyan, "[UPD ]"), ds.ID)

			var prevManifest []string
			if item != nil {
				prevManifest = item.DirPaths
			}
			claimed := claimedPaths(allDatasets, store, ds.ID, ds.Target)

			// Fetch (and re-fingerprint) from the first source that succeeds at both steps
			result, err := sourceAttempt(w, res, ds.ID, sources, fetchAttempt(ctx, ds.Target, prevManifest, claimed))
			if err != nil {
				var msg string
				if len(sources) > 1 {
					msg = fmt.Sprintf("all %d sources failed to fetch, last error: %v", len(sources), err)
				} else {
					msg = fmt.Sprintf("fetch: %v", err)
				}
				fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiRed, "[ERR ]"), ds.ID, msg)
				fmt.Fprintf(w, "%s %s: source may be inaccessible - please verify the source configuration\n", colorize(ansiBlue, "[INFO]"), ds.ID)
				failed := store.ensure(ds.ID)
				failed.InaccessibleAt = &now
				failed.InaccessibleError = err.Error()
				if res != nil {
					res.Status = StatusError
					res.Message = msg
					res.LockFingerprint = lockfp
				}
				return 1
			}
			fp = result.fp

			// Update lockfile with new fingerprint and local hash
			// Clear inaccessible status since fetch succeeded
			h, err := HashPath(ds.Target)
			if err != nil {
				msg := fmt.Sprintf("local hash after fetch: %v", err)
				fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiYellow, "[WARN]"), ds.ID, msg)
				if res != nil {
					res.Warnings = append(res.Warnings, msg)
				}
			}
			store.set(ds.ID, &LockItem{LocalSHA256: h, RemoteFingerprint: fp, DirPaths: result.manifest, CheckedAt: &now, InaccessibleAt: nil, InaccessibleError: ""})
			if res != nil {
				res.Status = StatusUpdated
				res.LockFingerprint = lockfp
				res.RemoteFingerprint = fp
			}
		} else {
			// Remote hasn't changed - just update the lock timestamps
			updated := store.ensure(ds.ID)
			updated.LocalSHA256 = localHash
			updated.RemoteFingerprint = fp
			updated.CheckedAt = &now
			fmt.Fprintf(w, "%s %s: up-to-date\n", colorize(ansiGreen, "[OK  ]"), ds.ID)
			if res != nil {
				res.Status = StatusOK
				res.LockFingerprint = fp
				res.RemoteFingerprint = fp
			}
		}
		return 0

	case "log":
		// LOG policy: Report changes but don't fail or update
		if stale {
			writeFingerprintChange(w, colorize(ansiYellow, "[STALE]"), "        ", ds.ID, lockfp, fp)
			if res != nil {
				res.Status = StatusStale
				res.LockFingerprint = lockfp
				res.RemoteFingerprint = fp
			}
		} else {
			fmt.Fprintf(w, "%s %s: up-to-date\n", colorize(ansiGreen, "[OK  ]"), ds.ID)
			if res != nil {
				res.Status = StatusOK
				res.LockFingerprint = fp
				res.RemoteFingerprint = fp
			}
		}
		return 0

	case "fail":
		// FAIL policy: Exit with error if remote has changed (strict mode)
		if stale {
			writeFingerprintChange(w, colorize(ansiRed, "[FAIL]"), "       ", ds.ID, lockfp, fp)
			if res != nil {
				res.Status = StatusFail
				res.LockFingerprint = lockfp
				res.RemoteFingerprint = fp
			}
			return 1
		}
		fmt.Fprintf(w, "%s %s: up-to-date\n", colorize(ansiGreen, "[OK  ]"), ds.ID)
		if res != nil {
			res.Status = StatusOK
			res.LockFingerprint = fp
			res.RemoteFingerprint = fp
		}
		return 0

	default:
		// Unknown policy - treat as "fail" with a warning
		msg := fmt.Sprintf("unknown policy=%q (treating as 'fail')", policy)
		fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiYellow, "[WARN]"), ds.ID, msg)
		if res != nil {
			res.Status = StatusWarn
			res.Message = msg
			res.LockFingerprint = lockfp
			res.RemoteFingerprint = fp
		}
		if stale {
			return 1
		}
		return 0
	}
}

// Fetch downloads data from external sources and updates the lockfile.
//
// Unlike Check, Fetch always downloads the data regardless of whether it has changed.
// This is useful for:
//   - Initial setup (first time downloading datasets)
//   - Explicitly updating specific datasets after they've changed
//   - Refreshing data on demand
//
// Parameters:
//   - ctx: Controls cancellation/timeout for all handler operations across all datasets
//   - cfgPath: Path to the configuration file (.data.yaml)
//   - lockPath: Path to the lockfile (.data.lock.yaml)
//   - ids: List of dataset IDs to fetch (empty list = fetch all datasets)
//   - concurrency: Maximum number of datasets to process in parallel (1 = sequential)
//
// Returns:
//   - 0: All requested datasets fetched successfully
//   - 1: One or more datasets failed to fetch
//   - 2: Configuration error or unknown handler type
func Fetch(ctx context.Context, cfgPath, lockPath string, ids []string, concurrency int) int {
	// Load configuration file
	cfg, err := readConfig(cfgPath)
	if err != nil {
		return reportError("config error", err)
	}

	// Build a set of IDs to fetch (if specific IDs were requested)
	which := map[string]bool{}
	for _, id := range ids {
		which[id] = true
	}

	// Load lockfile (or create empty one if it doesn't exist)
	lk, err := readLock(lockPath)
	if err != nil {
		return reportError("lock error", err)
	}
	store := &lockStore{lk: lk}

	now := time.Now().UTC()

	// Only the selected datasets get a slot (and output), same as the original sequential skip.
	var selected []Dataset
	for _, ds := range cfg.Datasets {
		if len(which) > 0 && !which[ds.ID] {
			continue
		}
		selected = append(selected, ds)
	}

	// In JSON mode the text writer is discarded entirely - results[i] carries the structured
	// outcome instead.
	outputs := make([]string, len(selected))
	results := make([]Result, len(selected))
	exitCodes := make([]int, len(selected))
	runConcurrently(len(selected), concurrency, func(i int) {
		results[i].ID = selected[i].ID
		var out strings.Builder
		w := io.Writer(&out)
		if JSONOutput {
			w = io.Discard
		}
		exitCodes[i] = fetchOneDataset(ctx, w, &results[i], selected[i], store, now, cfg.Datasets)
		outputs[i] = out.String()
	})

	exit := 0
	for i := range exitCodes {
		if exitCodes[i] > exit {
			exit = exitCodes[i]
		}
	}

	// Write updated lockfile back to disk
	lk.Version = 1
	lk.LastChecked = &now
	lockErr := writeLock(lockPath, lk)
	if lockErr != nil && exit == 0 {
		exit = 1
	}

	if JSONOutput {
		printReport(results, lockErr)
	} else {
		for _, out := range outputs {
			fmt.Print(out)
		}
		if lockErr != nil {
			fmt.Printf("lock write error: %v\n", lockErr)
		}
	}
	return exit
}

// fetchOneDataset runs Fetch's per-dataset logic, writing progress lines to w and reading/writing
// this dataset's LockItem through store. Returns this dataset's contribution to the exit code.
//
// res is optional (nil unless the caller is building --json output); when non-nil it's populated
// with the same outcome the text lines describe, so the two output modes never disagree.
func fetchOneDataset(ctx context.Context, w io.Writer, res *Result, ds Dataset, store *lockStore, now time.Time, allDatasets []Dataset) int {
	sources := ds.GetSources()

	item := store.get(ds.ID)
	if skipped := skipIfDeleted(w, res, ds.ID, item); skipped {
		return 0
	}

	fmt.Fprintf(w, "%s %s\n", colorize(ansiCyan, "[FETCH]"), ds.ID)

	var prevManifest []string
	if item != nil {
		prevManifest = item.DirPaths
	}
	claimed := claimedPaths(allDatasets, store, ds.ID, ds.Target)

	// Fetch (and re-fingerprint) from the first source that succeeds at both steps
	result, err := sourceAttempt(w, res, ds.ID, sources, fetchAttempt(ctx, ds.Target, prevManifest, claimed))
	if err != nil {
		var msg string
		if len(sources) > 1 {
			msg = fmt.Sprintf("all %d sources failed, last error: %v", len(sources), err)
		} else {
			msg = fmt.Sprintf("fetch: %v", err)
		}
		fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiRed, "[ERR ]"), ds.ID, msg)
		fmt.Fprintf(w, "%s %s: source may be inaccessible - please verify the source configuration\n", colorize(ansiBlue, "[INFO]"), ds.ID)
		item := store.ensure(ds.ID)
		item.InaccessibleAt = &now
		item.InaccessibleError = err.Error()
		if res != nil {
			res.Status = StatusError
			res.Message = msg
		}
		return 1
	}

	// Compute local hash and update lockfile
	// Clear inaccessible status since fetch succeeded
	h, err := HashPath(ds.Target)
	if err != nil {
		msg := fmt.Sprintf("local hash after fetch: %v", err)
		fmt.Fprintf(w, "%s %s: %s\n", colorize(ansiYellow, "[WARN]"), ds.ID, msg)
		if res != nil {
			res.Warnings = append(res.Warnings, msg)
		}
	}
	store.set(ds.ID, &LockItem{LocalSHA256: h, RemoteFingerprint: result.fp, DirPaths: result.manifest, CheckedAt: &now, InaccessibleAt: nil, InaccessibleError: ""})
	if res != nil {
		res.Status = StatusFetched
		res.RemoteFingerprint = result.fp
	}
	return 0
}
