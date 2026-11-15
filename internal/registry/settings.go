package registry

import "sync"

// Settings are process-wide handler options set from the command line (see core.Options). They
// live here, next to the handler registry, so handlers can read them without importing core and
// without changing the Fetcher interfaces. The zero value is datum's default behaviour.
type Settings struct {
	// CacheDir, when non-empty, replaces the default cache root (XDG_CACHE_HOME or ~/.cache/datum)
	// for handlers that keep a cache, such as git (--cache-dir).
	CacheDir string
	// GitHTTPSOnly makes the git handler refuse any URL that isn't https:// and never build SSH
	// authentication, so no SSH agent or key is consulted (--git-https-only, --https-only).
	GitHTTPSOnly bool
}

var (
	settingsMu sync.RWMutex
	settings   Settings
)

// Configure replaces the current handler settings. The CLI calls it once before running a
// command; tests call it and restore the previous value with the returned function.
func Configure(s Settings) (restore func()) {
	settingsMu.Lock()
	previous := settings
	settings = s
	settingsMu.Unlock()
	return func() {
		settingsMu.Lock()
		settings = previous
		settingsMu.Unlock()
	}
}

// Current returns the handler settings in effect.
func Current() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settings
}
