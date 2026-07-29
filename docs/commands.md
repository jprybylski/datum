---
layout: default
title: Commands
nav_order: 4
---

# Commands

Both commands accept these flags:

- `--config` (default `.data.yaml`) - path to the config file. Only needed if yours isn't named
  the default.
- `--lock` (default `.data.lock.yaml`) - path to the lockfile. Same deal - only needed for a
  non-default name.
- `--timeout` (default `5m`) - overall deadline for the whole run, covering every dataset's
  handler operations (HTTP requests, git fetches, shell commands). A hung source can't block the
  process forever; when it fires, in-flight datasets fail with a `context deadline exceeded`
  error rather than the process hanging. Accepts Go duration syntax (`30s`, `5m`, `1h`); `0`
  disables it.
- `--concurrency` (default `1`, sequential) - maximum number of datasets processed in parallel.
  Output is always printed back in the order datasets appear in `.data.yaml`, regardless of which
  one finishes first, so logs read the same way at any concurrency level. Values below `1` are
  treated as `1`.
- `--no-color` - disable colorized status tags (`[OK  ]`, `[FAIL]`, etc.). Color is already
  suppressed automatically when stdout isn't a terminal (e.g. piped output or CI logs), or when
  the `NO_COLOR` environment variable is set to any non-empty value.
- `--json` - print a single JSON document (`{"results": [...]}`, one object per dataset with its
  `id`, `status`, fingerprints, and any warnings) instead of colorized text, for scripts and other
  programmatic consumers. Exit codes are unchanged.

```bash
datum --timeout 2m --concurrency 4 check

# Only needed when the config/lock files aren't named the defaults:
datum --config other.yaml --lock other.lock.yaml check

# Machine-readable output for scripting:
datum --json check
```

## `datum check`

Verifies all configured datasets against their recorded fingerprints.

```bash
datum check
```

**Exit codes:**
- `0` - All datasets are up-to-date
- `1` - One or more datasets have changed or failed verification
- `2` - Configuration error

**What happens:**
1. Loads your configuration and lockfile
2. For each dataset:
   - Computes the current remote fingerprint
   - Compares against the lockfile
   - Applies the configured policy
3. Updates the lockfile with verification timestamps

## `datum fetch`

Downloads data from external sources and updates the lockfile.

```bash
# Fetch all datasets
datum fetch

# Fetch specific datasets by ID
datum fetch dataset1 dataset2
```

**What happens:**
1. Downloads the specified datasets (or all if none specified)
2. Computes fingerprints
3. Saves files to the target locations
4. Updates the lockfile

Next: [Handlers]({{ '/handlers.html' | relative_url }}) for what each source type supports, or
[Examples]({{ '/examples.html' | relative_url }}) to see full working configs.
