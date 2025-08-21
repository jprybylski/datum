package core

import (
	"context"
	"fmt"
	"time"

	"example.com/pinup/internal/registry"
)

// Check verifies all datasets against lock per policy.
// Returns an exit code (0 ok, 1 failures, 2 config/usage error).
func Check(cfgPath, lockPath string) int {
	cfg, err := readConfig(cfgPath)
	if err != nil {
		fmt.Printf("config error: %v\n", err)
		return 2
	}
	lk, _ := readLock(lockPath)
	if lk.Items == nil {
		lk.Items = map[string]*LockItem{}
	}
	ctx := context.Background()
	now := time.Now().UTC()
	exit := 0

	for _, ds := range cfg.Datasets {
		policy := firstNonEmpty(ds.Policy, cfg.Defaults.Policy)
		f, ok := registry.Get(ds.Source.Type)
		if !ok {
			fmt.Printf("[WARN] %s: unknown source.type=%q\n", ds.ID, ds.Source.Type)
			if exit == 0 {
				exit = 2
			}
			continue
		}
		fp, err := f.Fingerprint(ctx, ds.Source)
		if err != nil {
			fmt.Printf("[ERR ] %s: fingerprint: %v\n", ds.ID, err)
			if exit == 0 {
				exit = 1
			}
			continue
		}
		item := lk.Items[ds.ID]
		localHash := ""
		if fileExists(ds.Target) {
			if h, err := hashFile(ds.Target); err == nil {
				localHash = h
			} else {
				fmt.Printf("[ERR ] %s: local hash: %v\n", ds.ID, err)
			}
		}
		stale := (item == nil) || (item.RemoteFingerprint != fp)

		switch policy {
		case "update":
			if stale || !fileExists(ds.Target) {
				fmt.Printf("[UPD ] %s: refreshing\n", ds.ID)
				if err := f.Fetch(ctx, ds.Source, ds.Target); err != nil {
					fmt.Printf("[ERR ] %s: fetch: %v\n", ds.ID, err)
					if exit == 0 {
						exit = 1
					}
					continue
				}
				h, _ := hashFile(ds.Target)
				lk.Items[ds.ID] = &LockItem{LocalSHA256: h, RemoteFingerprint: fp, CheckedAt: &now}
			} else {
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
			if stale {
				lockfp := "<nil>"
				if item != nil {
					lockfp = item.RemoteFingerprint
				}
				fmt.Printf("[STALE] %s: remote changed (lock=%q -> now=%q)\n", ds.ID, lockfp, fp)
			} else {
				fmt.Printf("[OK  ] %s: up-to-date\n", ds.ID)
			}
			if item == nil {
				item = &LockItem{}
				lk.Items[ds.ID] = item
			}
			item.LocalSHA256 = localHash
			item.RemoteFingerprint = fp
			item.CheckedAt = &now
		case "fail":
			if stale {
				lockfp := "<nil>"
				if item != nil {
					lockfp = item.RemoteFingerprint
				}
				fmt.Printf("[FAIL] %s: remote changed (lock=%q -> now=%q)\n", ds.ID, lockfp, fp)
				exit = 1
			} else {
				fmt.Printf("[OK  ] %s: up-to-date\n", ds.ID)
			}
			if item == nil {
				item = &LockItem{}
				lk.Items[ds.ID] = item
			}
			item.LocalSHA256 = localHash
			item.RemoteFingerprint = fp
			item.CheckedAt = &now
		default:
			fmt.Printf("[WARN] %s: unknown policy=%q (treating as 'fail')\n", ds.ID, policy)
			if stale {
				exit = 1
			}
		}
	}

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

// Fetch forces (re)fetch for listed ids; if none provided, fetches all.
func Fetch(cfgPath, lockPath string, ids []string) int {
	cfg, err := readConfig(cfgPath)
	if err != nil {
		fmt.Printf("config error: %v\n", err)
		return 2
	}
	which := map[string]bool{}
	for _, id := range ids {
		which[id] = true
	}

	lk, _ := readLock(lockPath)
	if lk.Items == nil {
		lk.Items = map[string]*LockItem{}
	}

	ctx := context.Background()
	now := time.Now().UTC()
	exit := 0

	for _, ds := range cfg.Datasets {
		if len(which) > 0 && !which[ds.ID] {
			continue
		}
		f, ok := registry.Get(ds.Source.Type)
		if !ok {
			fmt.Printf("[WARN] %s: unknown source.type=%q\n", ds.ID, ds.Source.Type)
			if exit == 0 {
				exit = 2
			}
			continue
		}
		fmt.Printf("[FETCH] %s\n", ds.ID)
		if err := f.Fetch(ctx, ds.Source, ds.Target); err != nil {
			fmt.Printf("[ERR ] %s: fetch: %v\n", ds.ID, err)
			if exit == 0 {
				exit = 1
			}
			continue
		}
		fp, err := f.Fingerprint(ctx, ds.Source)
		if err != nil {
			fmt.Printf("[ERR ] %s: fingerprint after fetch: %v\n", ds.ID, err)
			if exit == 0 {
				exit = 1
			}
			continue
		}
		h, _ := hashFile(ds.Target)
		lk.Items[ds.ID] = &LockItem{LocalSHA256: h, RemoteFingerprint: fp, CheckedAt: &now}
	}

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
