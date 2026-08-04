---
layout: default
title: Examples
nav_order: 6
---

# Examples

Complete working examples are available in the
[`examples/`](https://github.com/jprybylski/datum/tree/main/examples) directory of the repo -
each one is a real, runnable `.data.yaml` you can `cd` into and try.

## Example 1: HTTP Handler - Tracking CDC Growth Chart Data

From [`examples/basic/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/basic/.data.yaml):

```yaml
version: 1
defaults:
  policy: fail
  algo: sha256

datasets:
  - id: cdc_wtage
    desc: CDC weight-for-age 2–20y
    source:
      type: http
      url: https://www.cdc.gov/growthcharts/data/zscore/wtage.csv
    target: data/ref/wtage.csv
    policy: fail
```

This example fetches CDC reference data for weight-for-age charts. The `fail` policy ensures your
pipeline breaks if the upstream data changes unexpectedly.

**Try it:**
```bash
cd examples/basic
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch a live run</summary>

<img src="{{ '/assets/img/basic-fetch.gif' | relative_url }}" alt="Terminal recording of datum fetch followed by datum check against the basic HTTP example" width="600" loading="lazy">

</details>

## Example 2: Git Handler - Tracking Dependency Licenses

From [`examples/git-one-file/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/git-one-file/.data.yaml):

```yaml
version: 1
defaults:
  policy: fail
  algo: sha256

datasets:
  - id: google_uuid_license
    desc: "LICENSE from github.com/google/uuid (branch: master)"
    source:
      type: git
      url: https://github.com/google/uuid.git
      ref: master
      path: LICENSE
    target: data/ref/google_uuid_LICENSE.txt
    policy: fail
```

This example tracks the LICENSE file from a GitHub repository, useful for compliance tracking or
ensuring you're always using the correct license text.

**Try it:**
```bash
cd examples/git-one-file
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch a live run</summary>

<img src="{{ '/assets/img/git-one-file.gif' | relative_url }}" alt="Terminal recording of datum fetch pulling a single LICENSE file out of a git repository, then datum check confirming it's up to date" width="600" loading="lazy">

</details>

## Example 3: File Handler - Copying Local Files

From [`examples/file-copy/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/file-copy/.data.yaml):

```yaml
version: 1
defaults:
  policy: update
  algo: sha256

datasets:
  - id: local_config
    desc: Configuration from local path
    source:
      type: file
      path: source-config.json
    target: config/copied.json
    policy: update
```

Use the file handler to copy files from local paths or network shares, with automatic updates
when the source changes.

**Try it:**
```bash
cd examples/file-copy
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch a live run</summary>

<img src="{{ '/assets/img/file-copy.gif' | relative_url }}" alt="Terminal recording of datum fetch copying a local file, then the source file changing and datum check silently re-copying it under the update policy" width="600" loading="lazy">

</details>

## Example 4: Command Handler - System Information

From [`examples/command-system/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/command-system/.data.yaml):

```yaml
version: 1
defaults:
  policy: log
  algo: sha256

datasets:
  - id: system_info
    desc: Fetch system information using command
    source:
      type: command
      fingerprint_cmd: "date +%Y-%m-%d"
      fetch_cmd: "mkdir -p $(dirname {{dest}}) && uname -a > {{dest}}"
    target: data/system-info.txt
    policy: log
```

The command handler allows custom fetch logic using shell commands. This example captures system
information and uses a date-based fingerprint.

**Try it:**
```bash
cd examples/command-system
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch a live run</summary>

<img src="{{ '/assets/img/command-system.gif' | relative_url }}" alt="Terminal recording of datum fetch running a shell command to capture system information, then datum check confirming it's up to date" width="600" loading="lazy">

</details>

## Example 5: Multi-Source with Fallback

From [`examples/multi-source/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/multi-source/.data.yaml):

```yaml
version: 1
datasets:
  # Fallback from primary HTTP source to backup HTTP source
  - id: cdc_wtage_with_backup
    desc: CDC weight-for-age data with fallback source
    sources:
      - type: http
        url: https://www.cdc.gov/growthcharts/data/zscore/wtage.csv
      - type: http
        url: https://example.com/backup/wtage.csv
    target: data/wtage.csv
    policy: fail

  # Fallback from HTTP to local file
  - id: config_with_local_fallback
    desc: Configuration file with local fallback
    sources:
      - type: http
        url: https://config.example.com/app-config.json
      - type: file
        path: ./backups/app-config.json
    target: data/app-config.json
    policy: update
```

This example demonstrates multi-source functionality where datum automatically falls back to
alternative sources if the primary source fails. This is useful for high availability, geographic
redundancy, and offline development workflows.

**Try it:**
```bash
cd examples/multi-source
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch a live run</summary>

This recording uses a scratch config with an unreachable HTTP source (a closed local port) and a
local file fallback, rather than `examples/multi-source` itself, whose `.data.yaml` uses
illustrative but unreachable mirror URLs (`example.com`, `mirror1-3.example.com`) - fine to read
as sample config, but not something a recording can run against deterministically. The behavior
shown is the same: `datum fetch` logs a `[WARN]` for the failed source and falls back to the
second one.

<img src="{{ '/assets/img/multi-source.gif' | relative_url }}" alt="Terminal recording of datum fetch failing over from an unreachable HTTP source to a local file fallback, then datum check confirming it's up to date" width="600" loading="lazy">

</details>

## Example 6: Multiple Datasets with Different Policies

From [`examples/multi-policy/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/multi-policy/.data.yaml):

```yaml
version: 1
defaults:
  policy: fail
  algo: sha256

datasets:
  # Critical reference data - fail if changed
  - id: cdc_wtage
    desc: CDC weight-for-age 2–20y
    source:
      type: http
      url: https://www.cdc.gov/growthcharts/data/zscore/wtage.csv
    target: data/wtage.csv
    policy: fail

  # Auto-update documentation
  - id: uuid_license
    desc: Google UUID library license
    source:
      type: git
      url: https://github.com/google/uuid.git
      ref: master
      path: LICENSE
    target: docs/licenses/uuid-LICENSE.txt
    policy: update

  # Monitor for changes
  - id: uuid_readme
    desc: Google UUID readme
    source:
      type: git
      url: https://github.com/google/uuid.git
      ref: master
      path: README.md
    target: docs/uuid-README.md
    policy: log
```

This example demonstrates using different policies for different types of data: strict
verification for critical data, automatic updates for documentation, and monitoring-only for
informational tracking.

**Try it:**
```bash
cd examples/multi-policy
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch the same change hit `fail`, `update`, and `log`</summary>

To isolate what each policy actually does, this recording uses a simplified stand-in config -
one local file tracked three times, once per policy - rather than
`examples/multi-policy` itself (whose three datasets track three unrelated sources). After the
file changes, `datum check` reacts three different ways in one run: `fail` blocks with `[FAIL]`,
`update` silently re-pins with `[UPD ]`, and `log` reports it with `[STALE]` and moves on.

<img src="{{ '/assets/img/policy-reactions.gif' | relative_url }}" alt="Terminal recording showing datum check reacting differently to the same file change under fail, update, and log policies" width="600" loading="lazy">

</details>

## Example 7: File Handler - Directory Sync

From [`examples/directory-sync/.data.yaml`](https://github.com/jprybylski/datum/blob/main/examples/directory-sync/.data.yaml):

```yaml
version: 1
defaults:
  policy: update
  algo: sha256

datasets:
  - id: directory_dataset
    desc: "Entire source-data/ directory tracked and synced as one dataset"
    source:
      type: file
      path: source-data
    target: synced-data
    policy: update
```

Pointing the file handler's `path` at a directory instead of a single file tracks the whole tree
as one dataset: every file gets recreated under `target`, and files removed from `source-data/`
are removed from `synced-data/` on the next fetch. See
[Directory sources]({{ '/handlers.html' | relative_url }}#directory-sources) for details.

**Try it:**
```bash
cd examples/directory-sync
datum fetch
datum check
```

<details markdown="1">
<summary>🎬 Watch a live run, including a deleted file dropping out of sync</summary>

<img src="{{ '/assets/img/directory-sync.gif' | relative_url }}" alt="Terminal recording of datum fetch syncing a directory, then a file being deleted from the source and a second fetch removing it from the synced target too" width="600" loading="lazy">

</details>
