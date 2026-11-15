package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jprybylski/datum/internal/registry"
)

// Options carries the opt-in behaviour a caller (usually the CLI's global flags) asks for. The zero
// value, with Concurrency 1, is exactly datum's behaviour without any of these flags, so wrappers
// that never pass them see no change.
//
// Most fields exist for callers that run datum on configs they didn't write, such as a local app
// that runs datum on a user's behalf, CI jobs over shared project folders, or wrappers like datur.
// See docs/embedding.md.
type Options struct {
	// Concurrency is the maximum number of datasets processed in parallel (--concurrency).
	Concurrency int

	// NoVCS turns off working-copy detection and ignore-rule edits entirely: datum never starts
	// git or svn and never writes .gitignore or svn:ignore (--no-vcs, DATUM_NO_VCS=1).
	NoVCS bool

	// AllowTypes, when non-empty, is the set of source types a config may use; any other type in
	// any dataset's source or sources list is a config error (--allow-types, DATUM_ALLOW_TYPES).
	AllowTypes []string

	// Root confines targets: each must be relative, contain no "..", and stay inside Root after
	// following symlinks. Relative targets and relative file source paths resolve against Root
	// instead of the working directory (--root).
	Root string
	// TargetPrefix, relative to Root (or the working directory), is a folder every target must
	// lie under (--target-prefix).
	TargetPrefix string

	// ReportOnly makes check compute and report fingerprints without writing anything: no
	// targets, no lockfile, no ignore rules. "update" datasets that would be re-fetched are
	// reported as stale (--report-only).
	ReportOnly bool

	// StageDir and LockOut make fetch write each target to StageDir/<target> and the new
	// lockfile to LockOut, leaving the real targets and lockfile untouched (--stage-dir,
	// --lock-out).
	StageDir string
	LockOut  string

	// EnvAllow and EnvPrefix restrict which ${NAME} references a config may use. When either is
	// set, a reference to any other name is a config error (--env-allow, --env-prefix).
	EnvAllow  []string
	EnvPrefix string

	// HTTPSOnly refuses http and git sources whose URL isn't https:// (--https-only).
	HTTPSOnly bool
	// GitHTTPSOnly refuses git sources whose URL isn't https:// and never consults an SSH agent
	// or key (--git-https-only).
	GitHTTPSOnly bool
	// CacheDir replaces the default cache root for handlers that keep a cache (--cache-dir).
	CacheDir string

	// VerifyLocal makes audit hash each tracked local file and report whether it still matches
	// the lockfile (--verify-local).
	VerifyLocal bool
}

// OptionsFromEnv returns the options datum also accepts from the environment: DATUM_NO_VCS and
// DATUM_ALLOW_TYPES. Command-line flags are applied on top by the caller.
func OptionsFromEnv() Options {
	var o Options
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DATUM_NO_VCS"))) {
	case "1", "true", "yes":
		o.NoVCS = true
	}
	o.AllowTypes = SplitList(os.Getenv("DATUM_ALLOW_TYPES"))
	return o
}

// SplitList splits a comma-separated flag value, trimming spaces and dropping empty items.
func SplitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func (o Options) concurrency() int {
	if o.Concurrency < 1 {
		return 1
	}
	return o.Concurrency
}

// applySettings hands the handler-level options to the registry for the duration of a command.
func (o Options) applySettings() (restore func()) {
	return registry.Configure(registry.Settings{
		CacheDir:     o.CacheDir,
		GitHTTPSOnly: o.GitHTTPSOnly || o.HTTPSOnly,
	})
}

// staged reports whether this is a staged fetch, and checks the staging flags are consistent.
func (o Options) staged(lockPath string) (bool, error) {
	if o.StageDir == "" && o.LockOut == "" {
		return false, nil
	}
	if o.StageDir == "" || o.LockOut == "" {
		return false, fmt.Errorf("--stage-dir and --lock-out must be given together")
	}
	same, err := samePath(o.LockOut, lockPath)
	if err != nil {
		return false, err
	}
	if same {
		return false, fmt.Errorf("--lock-out must not be the lockfile itself")
	}
	base := o.Root
	if base == "" {
		base = "."
	}
	if same, err := samePath(o.StageDir, base); err != nil {
		return false, err
	} else if same {
		return false, fmt.Errorf("--stage-dir must not be the project folder itself")
	}
	return true, nil
}

func samePath(a, b string) (bool, error) {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	return filepath.Clean(absA) == filepath.Clean(absB), nil
}
