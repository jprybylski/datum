package core

import (
	"context"
	"fmt"
	"time"

	"github.com/jprybylski/datum/internal/registry"
)

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
//   - cfgPath: Path to the configuration file (.data.yaml)
//   - lockPath: Path to the lockfile (.data.lock.yaml)
//
// Returns:
//   - 0: All datasets are up-to-date (success)
//   - 1: One or more datasets failed verification or had fetch errors
//   - 2: Configuration error or unknown handler type
//
// Go learning note: This function demonstrates error handling with exit codes,
// similar to Unix command conventions. The main() function will pass this to os.Exit().
func Check(cfgPath, lockPath string) int {
	// Load configuration file
	cfg, err := readConfig(cfgPath)
	if err != nil {
		fmt.Printf("config error: %v\n", err)
		return 2
	}

	// Load lockfile (or create empty one if it doesn't exist)
	lk, _ := readLock(lockPath)
	if lk.Items == nil {
		lk.Items = map[string]*LockItem{}
	}

	// Create context for handler operations (enables timeout/cancellation)
	ctx := context.Background()
	now := time.Now().UTC()
	exit := 0 // Track highest severity exit code

	// Process each dataset defined in the configuration
	for _, ds := range cfg.Datasets {
		// Determine which policy to use (dataset-specific or default)
		policy := firstNonEmpty(ds.Policy, cfg.Defaults.Policy)

		// Look up the handler for this source type (http, file, git, command)
		f, ok := registry.Get(ds.Source.Type)
		if !ok {
			fmt.Printf("[WARN] %s: unknown source.type=%q\n", ds.ID, ds.Source.Type)
			if exit == 0 {
				exit = 2 // Configuration error
			}
			continue
		}

		// Compute the current remote fingerprint
		// Different handlers use different strategies (ETag, file hash, git SHA, etc.)
		fp, err := f.Fingerprint(ctx, ds.Source)
		if err != nil {
			fmt.Printf("[ERR ] %s: fingerprint: %v\n", ds.ID, err)
			if exit == 0 {
				exit = 1 // Operational error
			}
			continue
		}

		// Get the lock entry for this dataset (may be nil if this is the first run)
		item := lk.Items[ds.ID]

		// Compute local file hash if the file exists
		localHash := ""
		if fileExists(ds.Target) {
			if h, err := HashFile(ds.Target); err == nil {
				localHash = h
			} else {
				fmt.Printf("[ERR ] %s: local hash: %v\n", ds.ID, err)
			}
		}

		// Determine if the remote source has changed since last check
		// It's stale if we have no lock entry, or if the fingerprint differs
		stale := (item == nil) || (item.RemoteFingerprint != fp)

		// Apply the policy based on whether the remote is stale
		switch policy {
		case "update":
			// UPDATE policy: Automatically fetch if remote changed or local file is missing
			if stale || !fileExists(ds.Target) {
				fmt.Printf("[UPD ] %s: refreshing\n", ds.ID)
				if err := f.Fetch(ctx, ds.Source, ds.Target); err != nil {
					fmt.Printf("[ERR ] %s: fetch: %v\n", ds.ID, err)
					fmt.Printf("[INFO] %s: source may be inaccessible - please verify the source configuration\n", ds.ID)
					// Record the failure in the lock file
					if item == nil {
						item = &LockItem{}
						lk.Items[ds.ID] = item
					}
					item.InaccessibleAt = &now
					item.InaccessibleError = err.Error()
					if exit == 0 {
						exit = 1
					}
					continue
				}
				// Update lockfile with new fingerprint and local hash
				// Clear inaccessible status since fetch succeeded
				h, _ := HashFile(ds.Target)
				lk.Items[ds.ID] = &LockItem{LocalSHA256: h, RemoteFingerprint: fp, CheckedAt: &now, InaccessibleAt: nil, InaccessibleError: ""}
			} else {
				// Remote hasn't changed - just update the lock timestamps
				if item == nil {
					item = &LockItem{}
					lk.Items[ds.ID] = item
				}
				item.LocalSHA256 = localHash
				item.RemoteFingerprint = fp
				item.CheckedAt = &now
				fmt.Printf("[OK  ] %s: up-to-date\n", ds.ID)
			}

		case "log":
			// LOG policy: Report changes but don't fail or update
			if stale {
				lockfp := "<nil>"
				if item != nil {
					lockfp = item.RemoteFingerprint
				}
				fmt.Printf("[STALE] %s: remote changed (lock=%q -> now=%q)\n", ds.ID, lockfp, fp)
			} else {
				fmt.Printf("[OK  ] %s: up-to-date\n", ds.ID)
			}
			// Don't update the lock - we want to keep reporting stale status until actually updated

		case "fail":
			// FAIL policy: Exit with error if remote has changed (strict mode)
			if stale {
				lockfp := "<nil>"
				if item != nil {
					lockfp = item.RemoteFingerprint
				}
				fmt.Printf("[FAIL] %s: remote changed (lock=%q -> now=%q)\n", ds.ID, lockfp, fp)
				exit = 1 // Mark as failed, but continue checking other datasets
			} else {
				fmt.Printf("[OK  ] %s: up-to-date\n", ds.ID)
			}
			// Don't update the lock - we want to keep failing until actually updated

		default:
			// Unknown policy - treat as "fail" with a warning
			fmt.Printf("[WARN] %s: unknown policy=%q (treating as 'fail')\n", ds.ID, policy)
			if stale {
				exit = 1
			}
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

// Fetch downloads data from external sources and updates the lockfile.
//
// Unlike Check, Fetch always downloads the data regardless of whether it has changed.
// This is useful for:
//   - Initial setup (first time downloading datasets)
//   - Explicitly updating specific datasets after they've changed
//   - Refreshing data on demand
//
// Parameters:
//   - cfgPath: Path to the configuration file (.data.yaml)
//   - lockPath: Path to the lockfile (.data.lock.yaml)
//   - ids: List of dataset IDs to fetch (empty list = fetch all datasets)
//
// Returns:
//   - 0: All requested datasets fetched successfully
//   - 1: One or more datasets failed to fetch
//   - 2: Configuration error or unknown handler type
//
// Go learning note: The ids parameter is a slice (dynamic array). Passing an empty
// slice vs. nil slice doesn't matter here - we check length with len(which) > 0.
func Fetch(cfgPath, lockPath string, ids []string) int {
	// Load configuration file
	cfg, err := readConfig(cfgPath)
	if err != nil {
		fmt.Printf("config error: %v\n", err)
		return 2
	}

	// Build a set of IDs to fetch (if specific IDs were requested)
	// Go learning note: Using a map[string]bool as a "set" is a common Go idiom.
	// Map lookup is O(1), making it efficient for membership testing.
	which := map[string]bool{}
	for _, id := range ids {
		which[id] = true
	}

	// Load lockfile (or create empty one if it doesn't exist)
	lk, _ := readLock(lockPath)
	if lk.Items == nil {
		lk.Items = map[string]*LockItem{}
	}

	// Create context for handler operations
	ctx := context.Background()
	now := time.Now().UTC()
	exit := 0 // Track highest severity exit code

	// Process each dataset (or just the requested ones)
	for _, ds := range cfg.Datasets {
		// Skip datasets not in the requested set (if IDs were specified)
		// If len(which) == 0, fetch all datasets
		if len(which) > 0 && !which[ds.ID] {
			continue
		}

		// Look up the handler for this source type
		f, ok := registry.Get(ds.Source.Type)
		if !ok {
			fmt.Printf("[WARN] %s: unknown source.type=%q\n", ds.ID, ds.Source.Type)
			if exit == 0 {
				exit = 2
			}
			continue
		}

		// Fetch the data from the source
		fmt.Printf("[FETCH] %s\n", ds.ID)
		if err := f.Fetch(ctx, ds.Source, ds.Target); err != nil {
			fmt.Printf("[ERR ] %s: fetch: %v\n", ds.ID, err)
			fmt.Printf("[INFO] %s: source may be inaccessible - please verify the source configuration\n", ds.ID)
			// Record the failure in the lock file
			item := lk.Items[ds.ID]
			if item == nil {
				item = &LockItem{}
				lk.Items[ds.ID] = item
			}
			item.InaccessibleAt = &now
			item.InaccessibleError = err.Error()
			if exit == 0 {
				exit = 1
			}
			continue
		}

		// Compute fingerprint after fetching
		// This ensures we record the exact state of what we just fetched
		fp, err := f.Fingerprint(ctx, ds.Source)
		if err != nil {
			fmt.Printf("[ERR ] %s: fingerprint after fetch: %v\n", ds.ID, err)
			if exit == 0 {
				exit = 1
			}
			continue
		}

		// Compute local file hash and update lockfile
		// Clear inaccessible status since fetch succeeded
		h, _ := HashFile(ds.Target)
		lk.Items[ds.ID] = &LockItem{LocalSHA256: h, RemoteFingerprint: fp, CheckedAt: &now, InaccessibleAt: nil, InaccessibleError: ""}
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
