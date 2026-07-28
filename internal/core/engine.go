package core

import (
	"context"
	"fmt"
	"io"
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
func sourceAttempt(w io.Writer, dsID string, sources []registry.Source, attempt func(f registry.Fetcher, source registry.Source) (value, warnLabel string, err error)) (string, error) {
	var lastErr error
	for i, source := range sources {
		f, ok := registry.Get(source.Type)
		if !ok {
			lastErr = fmt.Errorf("unknown source.type=%q", source.Type)
			if len(sources) > 1 {
				fmt.Fprintf(w, "[WARN] %s: source %d/%d: %v (trying next source)\n", dsID, i+1, len(sources), lastErr)
			}
			continue
		}

		value, warnLabel, err := attempt(f, source)
		if err != nil {
			lastErr = err
			if len(sources) > 1 {
				if warnLabel != "" {
					fmt.Fprintf(w, "[WARN] %s: source %d/%d: %s: %v (trying next source)\n", dsID, i+1, len(sources), warnLabel, err)
				} else {
					fmt.Fprintf(w, "[WARN] %s: source %d/%d: %v (trying next source)\n", dsID, i+1, len(sources), err)
				}
			}
			continue
		}

		return value, nil
	}
	return "", lastErr
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

// fetchAttempt builds a sourceAttempt callback that fetches into dest, then re-fingerprints the
// source so the lockfile records the fingerprint that actually corresponds to what was fetched.
// A source only counts as succeeded if both steps succeed.
func fetchAttempt(ctx context.Context, dest string) func(registry.Fetcher, registry.Source) (string, string, error) {
	return func(f registry.Fetcher, source registry.Source) (string, string, error) {
		if err := f.Fetch(ctx, source, dest); err != nil {
			return "", "fetch", err
		}
		fp, err := f.Fingerprint(ctx, source)
		if err != nil {
			return "", "fingerprint after fetch", err
		}
		return fp, "", nil
	}
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
		fmt.Printf("config error: %v\n", err)
		return 2
	}

	// Load lockfile (or create empty one if it doesn't exist)
	lk, err := readLock(lockPath)
	if err != nil {
		fmt.Printf("lock error: %v\n", err)
		return 2
	}
	store := &lockStore{lk: lk}

	now := time.Now().UTC()

	// Process each dataset, buffering its output so results print back in the original
	// config-file order regardless of how goroutines actually interleave/complete.
	outputs := make([]string, len(cfg.Datasets))
	exitCodes := make([]int, len(cfg.Datasets))
	runConcurrently(len(cfg.Datasets), concurrency, func(i int) {
		ds := cfg.Datasets[i]
		policy := firstNonEmpty(ds.Policy, cfg.Defaults.Policy)
		var out strings.Builder
		exitCodes[i] = checkOneDataset(ctx, &out, ds, policy, store, now)
		outputs[i] = out.String()
	})

	exit := 0
	for i := range outputs {
		fmt.Print(outputs[i])
		if exitCodes[i] > exit {
			exit = exitCodes[i]
		}
	}

	// Write updated lockfile back to disk
	lk.Version = 1
	lk.LastChecked = &now
	if err := writeLock(lockPath, lk); err != nil {
		fmt.Printf("lock write error: %v\n", err)
		if exit == 0 {
			exit = 1
		}
	}
	return exit
}

// checkOneDataset runs Check's per-dataset logic, writing progress lines to w and reading/writing
// this dataset's LockItem through store. Returns this dataset's contribution to the exit code.
func checkOneDataset(ctx context.Context, w io.Writer, ds Dataset, policy string, store *lockStore, now time.Time) int {
	sources := ds.GetSources()

	// Compute the current remote fingerprint, trying each source in order until one succeeds
	fp, err := sourceAttempt(w, ds.ID, sources, fingerprintAttempt(ctx))
	if err != nil {
		if len(sources) > 1 {
			fmt.Fprintf(w, "[ERR ] %s: all %d sources failed, last error: %v\n", ds.ID, len(sources), err)
		} else {
			fmt.Fprintf(w, "[ERR ] %s: fingerprint: %v\n", ds.ID, err)
		}
		return 1
	}

	// Get the lock entry for this dataset (may be nil if this is the first run)
	item := store.get(ds.ID)

	// Compute local hash if the target exists (file or directory)
	localHash := ""
	if fileExists(ds.Target) {
		if h, err := HashPath(ds.Target); err == nil {
			localHash = h
		} else {
			fmt.Fprintf(w, "[ERR ] %s: local hash: %v\n", ds.ID, err)
		}
	}

	// Determine if the remote source has changed since last check
	stale := (item == nil) || (item.RemoteFingerprint != fp)

	switch policy {
	case "update":
		// UPDATE policy: Automatically fetch if remote changed or local target is missing
		if stale || !fileExists(ds.Target) {
			fmt.Fprintf(w, "[UPD ] %s: refreshing\n", ds.ID)

			// Fetch (and re-fingerprint) from the first source that succeeds at both steps
			newFp, err := sourceAttempt(w, ds.ID, sources, fetchAttempt(ctx, ds.Target))
			if err != nil {
				if len(sources) > 1 {
					fmt.Fprintf(w, "[ERR ] %s: all %d sources failed to fetch, last error: %v\n", ds.ID, len(sources), err)
				} else {
					fmt.Fprintf(w, "[ERR ] %s: fetch: %v\n", ds.ID, err)
				}
				fmt.Fprintf(w, "[INFO] %s: source may be inaccessible - please verify the source configuration\n", ds.ID)
				failed := store.ensure(ds.ID)
				failed.InaccessibleAt = &now
				failed.InaccessibleError = err.Error()
				return 1
			}
			fp = newFp

			// Update lockfile with new fingerprint and local hash
			// Clear inaccessible status since fetch succeeded
			h, err := HashPath(ds.Target)
			if err != nil {
				fmt.Fprintf(w, "[WARN] %s: local hash after fetch: %v\n", ds.ID, err)
			}
			store.set(ds.ID, &LockItem{LocalSHA256: h, RemoteFingerprint: fp, CheckedAt: &now, InaccessibleAt: nil, InaccessibleError: ""})
		} else {
			// Remote hasn't changed - just update the lock timestamps
			updated := store.ensure(ds.ID)
			updated.LocalSHA256 = localHash
			updated.RemoteFingerprint = fp
			updated.CheckedAt = &now
			fmt.Fprintf(w, "[OK  ] %s: up-to-date\n", ds.ID)
		}
		return 0

	case "log":
		// LOG policy: Report changes but don't fail or update
		if stale {
			lockfp := "<nil>"
			if item != nil {
				lockfp = item.RemoteFingerprint
			}
			fmt.Fprintf(w, "[STALE] %s: remote changed (lock=%q -> now=%q)\n", ds.ID, lockfp, fp)
		} else {
			fmt.Fprintf(w, "[OK  ] %s: up-to-date\n", ds.ID)
		}
		return 0

	case "fail":
		// FAIL policy: Exit with error if remote has changed (strict mode)
		if stale {
			lockfp := "<nil>"
			if item != nil {
				lockfp = item.RemoteFingerprint
			}
			fmt.Fprintf(w, "[FAIL] %s: remote changed (lock=%q -> now=%q)\n", ds.ID, lockfp, fp)
			return 1
		}
		fmt.Fprintf(w, "[OK  ] %s: up-to-date\n", ds.ID)
		return 0

	default:
		// Unknown policy - treat as "fail" with a warning
		fmt.Fprintf(w, "[WARN] %s: unknown policy=%q (treating as 'fail')\n", ds.ID, policy)
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
		fmt.Printf("config error: %v\n", err)
		return 2
	}

	// Build a set of IDs to fetch (if specific IDs were requested)
	which := map[string]bool{}
	for _, id := range ids {
		which[id] = true
	}

	// Load lockfile (or create empty one if it doesn't exist)
	lk, err := readLock(lockPath)
	if err != nil {
		fmt.Printf("lock error: %v\n", err)
		return 2
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

	outputs := make([]string, len(selected))
	exitCodes := make([]int, len(selected))
	runConcurrently(len(selected), concurrency, func(i int) {
		var out strings.Builder
		exitCodes[i] = fetchOneDataset(ctx, &out, selected[i], store, now)
		outputs[i] = out.String()
	})

	exit := 0
	for i := range outputs {
		fmt.Print(outputs[i])
		if exitCodes[i] > exit {
			exit = exitCodes[i]
		}
	}

	// Write updated lockfile back to disk
	lk.Version = 1
	lk.LastChecked = &now
	if err := writeLock(lockPath, lk); err != nil {
		fmt.Printf("lock write error: %v\n", err)
		if exit == 0 {
			exit = 1
		}
	}
	return exit
}

// fetchOneDataset runs Fetch's per-dataset logic, writing progress lines to w and reading/writing
// this dataset's LockItem through store. Returns this dataset's contribution to the exit code.
func fetchOneDataset(ctx context.Context, w io.Writer, ds Dataset, store *lockStore, now time.Time) int {
	sources := ds.GetSources()

	fmt.Fprintf(w, "[FETCH] %s\n", ds.ID)

	// Fetch (and re-fingerprint) from the first source that succeeds at both steps
	fp, err := sourceAttempt(w, ds.ID, sources, fetchAttempt(ctx, ds.Target))
	if err != nil {
		if len(sources) > 1 {
			fmt.Fprintf(w, "[ERR ] %s: all %d sources failed, last error: %v\n", ds.ID, len(sources), err)
		} else {
			fmt.Fprintf(w, "[ERR ] %s: fetch: %v\n", ds.ID, err)
		}
		fmt.Fprintf(w, "[INFO] %s: source may be inaccessible - please verify the source configuration\n", ds.ID)
		item := store.ensure(ds.ID)
		item.InaccessibleAt = &now
		item.InaccessibleError = err.Error()
		return 1
	}

	// Compute local hash and update lockfile
	// Clear inaccessible status since fetch succeeded
	h, err := HashPath(ds.Target)
	if err != nil {
		fmt.Fprintf(w, "[WARN] %s: local hash after fetch: %v\n", ds.ID, err)
	}
	store.set(ds.ID, &LockItem{LocalSHA256: h, RemoteFingerprint: fp, CheckedAt: &now, InaccessibleAt: nil, InaccessibleError: ""})
	return 0
}
