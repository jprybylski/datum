---
layout: default
title: Home
nav_order: 1
description: "Datum is a data pinning tool that tracks external data sources with cryptographic fingerprints."
permalink: /
---

# Datum

**Datum** is a data pinning tool that tracks external data sources with cryptographic
fingerprints. It helps ensure that your project's external dependencies (files, URLs, git
repositories) haven't changed unexpectedly.

Think of it as a "lockfile" for external data sources, similar to how `package-lock.json` or
`go.sum` work for code dependencies.
{: .fs-6 .fw-300 }

[Get started](#quick-start){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/jprybylski/datum){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## What Does Datum Do?

Datum provides two main capabilities:

1. **`check`** - Verifies that external data sources haven't changed since you last pinned them
2. **`fetch`** - Downloads external data and records their cryptographic fingerprints

This is useful for:
- Reproducible data pipelines
- Detecting when external APIs or files change
- Ensuring research data hasn't been modified
- Tracking specific files from git repositories

## Why Use Datum?

Imagine you're building a data analysis project that depends on external CSV files, JSON APIs, or
documentation from git repositories. Without datum:

- You don't know if the external data has changed
- Manual verification is time-consuming and error-prone
- Your analysis might break silently when upstream data changes

With datum:

- Automated verification of all external data sources
- Cryptographic fingerprints ensure data integrity
- Configurable policies (fail, update, or log when changes are detected)
- Single source of truth in your configuration file

Curious how this compares to DVC, pins, Pooch, Quilt, LakeFS, or Pachyderm? See
[Comparison with Other Tools]({{ '/comparison.html' | relative_url }}).

## Quick Start

### 1. Create a configuration file (`.data.yaml`)

```yaml
version: 1
defaults:
  policy: fail  # fail | update | log
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

### 2. Fetch the data

```bash
datum fetch
```

This downloads the file and creates a `.data.lock.yaml` with its fingerprint.

### 3. Verify data integrity

```bash
datum check
```

This checks if the remote data has changed. Based on your policy:
- **fail**: Exits with error code 1 if data changed
- **update**: Automatically downloads the new version
- **log**: Reports changes but doesn't fail

Ready for more? Head to [Installation]({{ '/installation.html' | relative_url }}) or jump straight to
[Configuration]({{ '/configuration.html' | relative_url }}).

## Where to next

| Page | What's there |
|---|---|
| [Installation]({{ '/installation.html' | relative_url }}) | Prerequisites, building from source, build tags |
| [Configuration]({{ '/configuration.html' | relative_url }}) | `.data.yaml` structure, multi-source fallback, policies, IDE schema support |
| [Commands]({{ '/commands.html' | relative_url }}) | `check` / `fetch` / `delete` / `undelete` / `unlock` / `audit`, exit codes, flags |
| [Handlers]({{ '/handlers.html' | relative_url }}) | HTTP, File (incl. directory sources), Command, and Git handlers |
| [Examples]({{ '/examples.html' | relative_url }}) | Seven complete, runnable example configurations |
| [Comparison]({{ '/comparison.html' | relative_url }}) | How Datum differs from DVC, pins, Pooch, Quilt, LakeFS, Pachyderm |
| [Architecture & Development]({{ '/architecture.html' | relative_url }}) | Project structure, adding a handler, running tests |
| [FAQ]({{ '/faq.html' | relative_url }}) | Common questions |
