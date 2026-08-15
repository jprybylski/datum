---
layout: default
title: Commands
nav_order: 4
---

# Commands

datum has seven subcommands: `check`, `fetch`, `delete`, `undelete`, `unlock`, `audit`, and
`types`. All of
them accept `--config` (default `.data.yaml`) and `--lock` (default `.data.lock.yaml`) - only
needed if yours aren't named the defaults - with one exception: `undelete` only ever touches the
lockfile, so it doesn't take `--config` at all.

Every flag goes *before* the subcommand (`datum --json check`, not `datum check --json`) - that's
a `flag`-package rule, not a datum-specific one, but it trips people up.

`check` and `fetch` additionally accept:

- `--timeout` (default `5m`) - overall deadline for the whole run, covering every dataset's
  handler operations (HTTP requests, git fetches, shell commands). A hung source can't block the
  process forever; when it fires, in-flight datasets fail with a `context deadline exceeded`
  error rather than the process hanging. Accepts Go duration syntax (`30s`, `5m`, `1h`); `0`
  disables it.
- `--concurrency` (default `1`, sequential) - maximum number of datasets processed in parallel.
  Output is always printed back in the order datasets appear in `.data.yaml`, regardless of which
  one finishes first, so logs read the same way at any concurrency level. Values below `1` are
  treated as `1`.

`check`, `fetch`, and `audit` additionally accept:

- `--no-color` - disable colorized status tags (`[OK  ]`, `[FAIL]`, etc.). Color is already
  suppressed automatically when stdout isn't a terminal (e.g. piped output or CI logs), or when
  the `NO_COLOR` environment variable is set to any non-empty value.
- `--json` - print a single JSON document instead of colorized text, for scripts and other
  programmatic consumers (`check`/`fetch`: `{"results": [...]}`, one object per dataset with its
  `id`, `status`, fingerprints, and any warnings; `audit`: `{"entries": [...]}`, see below). Exit
  codes are unchanged.

`delete` and `unlock` additionally accept:

- `--yes` - skip the confirmation prompt (for scripts/CI). Has no effect on other commands.

```bash
datum --timeout 2m --concurrency 4 check

# Only needed when the config/lock files aren't named the defaults:
datum --config other.yaml --lock other.lock.yaml check

# Machine-readable output for scripting:
datum --json check

# --yes goes before the subcommand too, like every other flag:
datum --yes delete some_id
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

## `datum delete`

Removes the local files tracked for one or more datasets and marks them deleted in the lockfile,
so later `check`/`fetch` runs skip them instead of treating the now-missing target as something to
re-fetch or fail on. It never touches `.data.yaml` - only the lockfile records the deletion - so
`datum undelete` can resume tracking without your config having drifted.

```bash
datum delete ID [ID ...]

# Skip the confirmation prompt (for scripts/CI) - note --yes goes before the subcommand,
# like the other flags above:
datum --yes delete ID [ID ...]
```

**What happens:**
1. Resolves each ID against your configuration; an unknown ID aborts before anything is deleted
2. Prints what will be removed and asks for confirmation (unless `--yes`)
3. For a single-file target, removes the file. For a directory-synced target, removes only the
   relative paths that dataset itself wrote (tracked via `dir_paths` in the lockfile), cleans up
   any subdirectories left empty, and removes the target directory itself only if no other
   dataset also targets it
4. Marks the dataset `deleted: true` in the lockfile (with a `deleted_at` timestamp)

**Exit codes:**
- `0` - Deleted (or nothing to do - already-deleted IDs are skipped with a note, and declining the
  confirmation prompt aborts cleanly)
- `1` - One or more datasets' files couldn't be removed, or the lockfile couldn't be written
- `2` - Configuration/lock error, no IDs given, or an ID isn't a known dataset

<details markdown="1">
<summary>🎬 Watch a live run</summary>

<img src="{{ '/assets/img/delete.gif' | relative_url }}" alt="Terminal recording of datum delete removing a tracked file with --yes, datum check skipping the deleted dataset, then datum undelete plus datum fetch restoring it" width="600" loading="lazy">

</details>

## `datum undelete`

Clears the `deleted` flag `datum delete` set for one or more datasets, so the next `check`/`fetch`
resumes treating them normally. It only edits the lockfile - it doesn't fetch data itself, so
follow it with `datum fetch ID` to actually restore the files.

```bash
datum undelete ID [ID ...]
```

**Exit codes:**
- `0` - Every ID was undeleted
- `1` - One or more IDs weren't marked deleted (including IDs datum has never heard of)
- `2` - Lock error, or no IDs given

## `datum unlock`

Permanently removes the lockfile entry for one or more IDs, whether or not they're still in
`.data.yaml`. Unlike `delete`, it never touches local files - it only forgets tracking history.
This is what you want for entries orphaned by removing a dataset from `.data.yaml` by hand:
`check`/`fetch` never prune those on their own, so they'd otherwise sit in the lockfile forever
(harmlessly, but `datum audit` will keep flagging them until you either restore the dataset or run
`unlock`).

Unlocking an ID that's still actively tracked in the config is allowed, but resets its pin - the
next `check` under a `fail` policy will report "remote changed" from `(none)`, since there's no
longer a recorded fingerprint to compare against. The confirmation prompt calls this out.

```bash
datum unlock ID [ID ...]

# Skip the confirmation prompt (for scripts/CI) - --yes goes before the subcommand,
# like the other flags above:
datum --yes unlock ID [ID ...]
```

**Exit codes:**
- `0` - Every ID was unlocked (or there was nothing to do), or the confirmation was declined
- `1` - One or more IDs had no lockfile entry, or the lockfile couldn't be written
- `2` - Lock error, or no IDs given

## `datum audit`

Reports every dataset id known to either `.data.yaml` or the lockfile, and how they relate - a
purely local, read-only view that never contacts a data source. Config datasets are listed first
in `.data.yaml` order; lockfile entries no longer in the config (orphaned) are appended after,
sorted by id.

Each entry is one of:
- `ok` - tracked in config, has a lock entry, not deleted
- `pending` - tracked in config, never fetched yet
- `deleted` - `datum delete` removed it (still shows whether it's still in the config or also
  orphaned)
- `orphaned` - has a lock entry but no longer appears in `.data.yaml`

```bash
datum audit

# Machine-readable output for scripting:
datum --json audit
```

`--no-color` and `NO_COLOR` are honored the same as `check`/`fetch`. Exit code is always `0`
unless the config or lockfile can't be loaded (`2`) - `audit` reports what it finds, it doesn't
judge it; that's what `check` is for.

<details markdown="1">
<summary>🎬 Watch a live run</summary>

<img src="{{ '/assets/img/audit.gif' | relative_url }}" alt="Terminal recording of datum audit reporting ok, pending, deleted, and orphaned datasets after a fetch, a delete, and a config edit, followed by datum --json audit" width="600" loading="lazy">

</details>

## `datum types`

Lists the dataset source types included in the current build. Pass one or more names to see every
required and optional source field for those types. The optional `git` type only appears in builds
that include Git support.

```bash
datum types
datum types http file

# Machine-readable output:
datum --json types
datum --json types command
```

`--no-color`, `NO_COLOR`, and `--json` are supported. An unknown type exits `2`; successful output
exits `0`.

Next: [Handlers]({{ '/handlers.html' | relative_url }}) for what each source type supports, or
[Examples]({{ '/examples.html' | relative_url }}) to see full working configs.
