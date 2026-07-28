---
layout: default
title: Commands
nav_order: 4
---

# Commands

Both commands accept two flags in addition to `--config`/`--lock`:

- `--timeout` (default `5m`) - overall deadline for the whole run, covering every dataset's
  handler operations (HTTP requests, git fetches, shell commands). A hung source can't block the
  process forever; when it fires, in-flight datasets fail with a `context deadline exceeded`
  error rather than the process hanging. Accepts Go duration syntax (`30s`, `5m`, `1h`); `0`
  disables it.
- `--concurrency` (default `1`, sequential) - maximum number of datasets processed in parallel.
  Output is always printed back in the order datasets appear in `.data.yaml`, regardless of which
  one finishes first, so logs read the same way at any concurrency level. Values below `1` are
  treated as `1`.

```bash
datum --config .data.yaml --timeout 2m --concurrency 4 check
```

## `datum check`

Verifies all configured datasets against their recorded fingerprints.

```bash
datum --config .data.yaml --lock .data.lock.yaml check
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
datum --config .data.yaml fetch

# Fetch specific datasets by ID
datum --config .data.yaml fetch dataset1 dataset2
```

**What happens:**
1. Downloads the specified datasets (or all if none specified)
2. Computes fingerprints
3. Saves files to the target locations
4. Updates the lockfile

Next: [Handlers]({{ '/handlers.html' | relative_url }}) for what each source type supports, or
[Examples]({{ '/examples.html' | relative_url }}) to see full working configs.
