# Datum

[![CI](https://github.com/jprybylski/datum/actions/workflows/ci.yml/badge.svg)](https://github.com/jprybylski/datum/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/jprybylski/datum)](https://github.com/jprybylski/datum/releases/latest)
[![codecov](https://codecov.io/gh/jprybylski/datum/branch/main/graph/badge.svg)](https://codecov.io/gh/jprybylski/datum)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jprybylski/datum)](https://github.com/jprybylski/datum/blob/main/go.mod)

**Datum** is a data pinning tool that tracks external data sources with cryptographic fingerprints. It helps ensure that your project's external dependencies (files, URLs, git repositories) haven't changed unexpectedly.

Think of it as a "lockfile" for external data sources, similar to how `package-lock.json` or `go.sum` work for code dependencies.

📖 **[Full documentation, examples, and guides →](https://jprybylski.github.io/datum/)** - this README covers the essentials; the docs site has everything else (tool comparisons, architecture, contributor guide, all seven examples in full).

## Table of Contents

- [What Does Datum Do?](#what-does-datum-do)
- [Why Use Datum?](#why-use-datum)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Commands](#commands)
- [Data Source Handlers](#data-source-handlers)
- [Examples](#examples)
- [FAQ](#faq)
- [AI Acknowledgment](#ai-acknowledgment)
- [License](#license)
- [Contributing](#contributing)

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

Imagine you're building a data analysis project that depends on external CSV files, JSON APIs, or documentation from git repositories. Without datum:

- You don't know if the external data has changed
- Manual verification is time-consuming and error-prone
- Your analysis might break silently when upstream data changes

With datum:

- Automated verification of all external data sources
- Cryptographic fingerprints ensure data integrity
- Configurable policies (fail, update, or log when changes are detected)
- Single source of truth in your configuration file

Wondering how this compares to DVC, pins, Pooch, Quilt, LakeFS, or Pachyderm? See the
[tool comparison](https://jprybylski.github.io/datum/comparison.html) on the docs site.

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

## Installation

### Prebuilt Binaries

Download a binary for Linux, macOS, or Windows (amd64/arm64) from the
[latest release](https://github.com/jprybylski/datum/releases/latest) - each one is built with
git support included. Verify against `checksums.txt` in the same release if you want.

### Prerequisites (building from source)

- Go 1.25 or later
- Git (if you want git repository support)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/jprybylski/datum.git
cd datum

# Build without git support (HTTP, file, command handlers only)
go mod tidy
go build ./cmd/datum

# Build with git support
go build -tags git ./cmd/datum

# Or use the build script
bash scripts/make.sh        # Linux/Mac
# or
pwsh scripts/make.ps1       # Windows
```

The binary will be in `./bin/datum` (or `./bin/datum.exe` on Windows). Cross-compiling for other
platforms and details on the build scripts: see
[Installation](https://jprybylski.github.io/datum/installation.html) on the docs site.

## Configuration

Datum uses two files, both version controlled:

1. **`.data.yaml`** - Your configuration
2. **`.data.lock.yaml`** - Generated lockfile with fingerprints

```yaml
version: 1                    # Config format version

defaults:
  policy: fail                # Default policy for all datasets
  algo: sha256                # Hashing algorithm (currently only sha256)

datasets:
  - id: unique_identifier     # Unique ID for this dataset
    desc: Human-readable description
    source:                   # Where to get the data (single source)
      type: http              # Handler type (http, file, git, command)
      url: https://...        # Handler-specific fields
    target: path/to/local/file.csv  # Where to save locally
    policy: update            # Override default policy (optional)
```

**Policy options:**
- **`fail`**: Verification fails if the remote data has changed (strict mode)
- **`update`**: Automatically fetch and update if the remote data has changed
- **`log`**: Log changes but don't fail or update (monitoring mode)

Datum also supports listing multiple `sources:` per dataset with automatic ordered fallback, and
a JSON Schema (`data-schema.json`) for IDE autocomplete/validation. See
[Configuration](https://jprybylski.github.io/datum/configuration.html) on the docs site for both.

## Commands

Both commands accept `--config` (default `.data.yaml`), `--lock` (default `.data.lock.yaml`),
`--timeout` (default `5m`, bounds the whole run), and `--concurrency` (default `1`, sequential;
processes datasets in parallel above that). You only need to pass `--config`/`--lock` if your
files aren't named the defaults.

```bash
# Verify all datasets against their recorded fingerprints
datum check

# Download data and update the lockfile (all datasets, or specific IDs)
datum fetch
datum fetch dataset1 dataset2
```

`check` exits `0` (up-to-date), `1` (changed/failed), or `2` (config error). Full flag reference
and exit-code details: [Commands](https://jprybylski.github.io/datum/commands.html).

## Data Source Handlers

Datum uses a plugin-based handler system - each handler knows how to fetch from one kind of
source. Full config examples and fingerprinting details for each are on the
[Handlers](https://jprybylski.github.io/datum/handlers.html) page.

| Handler | `source.type` | What it does |
|---|---|---|
| HTTP | `http` | Fetches a URL; fingerprints via ETag, then Last-Modified, then SHA256 |
| File | `file` | Copies a local file - or, if `path` is a directory, syncs the whole tree (with deletion tracking) |
| Command | `command` | Runs your own shell commands to fingerprint/fetch anything |
| Git *(requires `-tags git`)* | `git` | Fetches one file from a git repo by branch or tag, over HTTPS or SSH |

## Examples

Seven complete, runnable examples live in [`examples/`](examples/) - one per handler plus
multi-source fallback, multi-policy, and directory sync. Full walkthroughs of each:
[Examples](https://jprybylski.github.io/datum/examples.html).

```bash
cd examples/basic
datum fetch
datum check
```

## FAQ

### Why "datum"?

Datum is the singular form of "data" - fitting for a tool that manages individual data sources.

### Can I use this in CI/CD?

Yes! Use `datum check` in your CI pipeline to verify that external data hasn't changed unexpectedly.

### How do I version control the lockfile?

Yes, commit both `.data.yaml` and `.data.lock.yaml`. This gives your team the same data versions,
a historical record of when data changed, and reproducible builds.

More questions answered on the [FAQ](https://jprybylski.github.io/datum/faq.html) page.

## AI Acknowledgment

This project was developed with assistance from Claude (Anthropic's AI assistant). AI assistance was used for:

- Code generation and implementation
- Documentation writing and structuring
- Test case development
- Code review and refactoring suggestions
- Build script creation

All AI-generated content has been reviewed, tested, and modified by the project maintainer to ensure quality, correctness, and alignment with project goals.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our development process, coding standards, and how to submit pull requests. The
[Architecture & Development](https://jprybylski.github.io/datum/architecture.html) page covers
the codebase's structure, how to add a new handler, and how to run the test suite.
