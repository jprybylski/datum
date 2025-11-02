# Datum

[![CI](https://github.com/jprybylski/datum/actions/workflows/ci.yml/badge.svg)](https://github.com/jprybylski/datum/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/jprybylski/datum/branch/main/graph/badge.svg)](https://codecov.io/gh/jprybylski/datum)
[![Go Report Card](https://goreportcard.com/badge/github.com/jprybylski/datum?style=flat)](https://goreportcard.com/report/github.com/jprybylski/datum)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jprybylski/datum)](https://github.com/jprybylski/datum/blob/main/go.mod)

**Datum** is a data pinning tool that tracks external data sources with cryptographic fingerprints. It helps ensure that your project's external dependencies (files, URLs, git repositories) haven't changed unexpectedly.

Think of it as a "lockfile" for external data sources, similar to how `package-lock.json` or `go.sum` work for code dependencies.

## Table of Contents

- [What Does Datum Do?](#what-does-datum-do)
- [Why Use Datum?](#why-use-datum)
- [Comparison with Other Tools](#comparison-with-other-tools)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Commands](#commands)
- [Data Source Handlers](#data-source-handlers)
- [Architecture and Implementation](#architecture-and-implementation)
- [Project Structure](#project-structure)
- [Development Guide](#development-guide)
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

## Comparison with Other Tools

Datum fills a specific niche in the data versioning and management ecosystem. Here's how it compares to other popular tools:

### DVC (Data Version Control)

**DVC** is a comprehensive data versioning system that works alongside Git.

- **Scope**: DVC versions the data itself, storing it in remote storage (S3, GCS, Azure, etc.)
- **Complexity**: Requires cloud storage setup, Git integration, and learning DVC-specific commands
- **Use case**: Full data versioning with history, collaboration on large datasets, ML experiment tracking

**Datum** focuses on verification and fingerprinting, not storage:

- No cloud storage required - data stays where it is
- Simple lockfile approach (like `package-lock.json`)
- Designed for reproducibility checks in CI/CD pipelines
- Lightweight single binary with no dependencies

**When to use DVC**: You need full version history of datasets, team collaboration on data, or ML experiment tracking.

**When to use Datum**: You want to verify that external data sources haven't changed, track specific files from URLs/git repos, or ensure reproducible data pipelines without managing data storage.

### pins (R package)

**pins** is an R package for publishing and sharing data.

- **Language**: R-specific, requires R runtime
- **Focus**: Publishing data to shared locations (RStudio Connect, S3, Azure, local folders)
- **Versioning**: Stores multiple versions of datasets with automatic versioning
- **Use case**: Sharing datasets within R workflows and RStudio environments

**Datum** is language-agnostic and verification-focused:

- Works with any language or toolchain (Go binary, no R required)
- Doesn't store or publish data - tracks external sources
- Verifies data hasn't changed rather than managing versions
- Plugin-based handlers for different source types

**When to use pins**: You're working in R and need to share/publish versioned datasets within your R ecosystem.

**When to use Datum**: You need language-agnostic data verification, want to track external sources (not publish your own), or use multiple languages in your pipeline.

### Pooch (Python)

**Pooch** is a Python library for downloading and caching data files.

- **Language**: Python-specific
- **Focus**: Downloading files from the web and caching them locally
- **Verification**: Uses SHA256 hashes to verify downloads
- **Use case**: Scientific Python projects that need reliable data downloads

**Datum** offers broader functionality and language independence:

- Standalone binary (no Python required)
- Multiple source handlers (HTTP, git, file, command)
- Policy-based behavior (fail/update/log)
- Multi-source fallback support
- Designed for CI/CD integration

**When to use Pooch**: You have a Python project and need simple, reliable file downloads with caching.

**When to use Datum**: You need verification across multiple source types, language-independent operation, or integration with non-Python build systems.

### Quilt (Python)

**Quilt** is a data package manager for Python.

- **Focus**: Packaging datasets as versioned, reusable data packages
- **Storage**: Stores data in Quilt catalogs (S3-backed)
- **Features**: Data discovery, versioning, lineage tracking, metadata management
- **Use case**: Large organizations managing data catalogs and enabling data discovery

**Datum** is much simpler and focused:

- No centralized catalog or storage required
- Tracks external sources rather than creating data packages
- Lightweight verification without metadata overhead
- No commercial platform or S3 backend needed

**When to use Quilt**: You're building a data catalog for an organization, need data discovery features, or want to package datasets for reuse.

**When to use Datum**: You want lightweight verification of external dependencies without setting up infrastructure.

### LakeFS

**LakeFS** provides Git-like version control for object storage (S3, Azure Blob, GCS).

- **Scale**: Enterprise-grade, designed for data lakes
- **Features**: Branches, commits, merges for data, atomic operations, rollback
- **Infrastructure**: Requires LakeFS server and object storage
- **Use case**: Large-scale data lakes with multiple teams and complex workflows

**Datum** is intentionally minimal:

- No server or infrastructure required
- Single binary, single config file
- Verification-focused, not storage-focused
- Suitable for projects of any size

**When to use LakeFS**: You're managing a data lake and need Git-like operations on object storage.

**When to use Datum**: You need simple verification of external data sources without infrastructure overhead.

### Pachyderm

**Pachyderm** is a data pipeline platform with versioning.

- **Scope**: Full data pipeline orchestration with built-in versioning
- **Features**: DAG-based pipelines, automatic versioning, Kubernetes-native
- **Infrastructure**: Requires Kubernetes cluster
- **Use case**: Production data pipelines with complex dependencies and scalability needs

**Datum** is a single-purpose tool:

- Verification and fetching only - no pipeline orchestration
- Runs anywhere (no Kubernetes required)
- Complements existing build systems rather than replacing them
- Minimal resource footprint

**When to use Pachyderm**: You need a complete data pipeline platform with orchestration and scaling.

**When to use Datum**: You have an existing build system (Make, CI/CD) and just need data verification.

### Summary Table

| Tool       | Language  | Storage Required | Primary Focus        | Complexity | Best For                           |
|------------|-----------|------------------|----------------------|------------|------------------------------------|
| Datum      | Any       | No               | Verification/Pinning | Low        | CI/CD checks, reproducibility      |
| DVC        | Any       | Yes (cloud)      | Data versioning      | Medium     | ML projects, team collaboration    |
| pins       | R         | Optional         | Sharing datasets     | Low        | R workflows, RStudio ecosystem     |
| Pooch      | Python    | No               | Download/cache       | Low        | Scientific Python projects         |
| Quilt      | Python    | Yes (S3)         | Data catalog         | Medium     | Data discovery, organizational use |
| LakeFS     | Any       | Yes (object)     | Data lake versioning | High       | Enterprise data lakes              |
| Pachyderm  | Any       | Yes (K8s)        | Pipeline platform    | High       | Production data pipelines          |

### Datum's Sweet Spot

Datum excels when you need to:

- **Verify external dependencies** haven't changed (URLs, APIs, git repos)
- **Track specific files** from git repositories without cloning entire repos
- **Integrate with CI/CD** to catch unexpected data changes
- **Avoid infrastructure** overhead (no cloud storage, no servers, no containers)
- **Work across languages** in polyglot projects
- **Keep it simple** with a single config file and lockfile

Think of Datum as `package-lock.json` for data: it doesn't store your data or manage complex workflows, but it ensures your data dependencies are exactly what you expect them to be.

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
datum --config .data.yaml fetch
```

This downloads the file and creates a `.data.lock.yaml` with its fingerprint.

### 3. Verify data integrity

```bash
datum --config .data.yaml check
```

This checks if the remote data has changed. Based on your policy:
- **fail**: Exits with error code 1 if data changed
- **update**: Automatically downloads the new version
- **log**: Reports changes but doesn't fail

## Installation

### Prerequisites

- Go 1.23 or later
- Git (if you want git repository support)

### Build from Source

```bash
# Clone the repository
git clone <repository-url>
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

The binary will be in `./bin/datum` (or `./bin/datum.exe` on Windows).

### Build Scripts Explained

The project includes helper scripts in the `scripts/` directory:

- **`make.sh`** (Linux/Mac): Runs `go mod tidy`, `go vet`, and builds the binary
- **`make.ps1`** (Windows): Same as above, but for PowerShell

You can pass build tags as arguments:

```bash
# Build with git support
bash scripts/make.sh git
```

## Configuration

Datum uses two files:

1. **`.data.yaml`** - Your configuration (version controlled)
2. **`.data.lock.yaml`** - Generated lockfile with fingerprints (version controlled)

### IDE Support with JSON Schema

Datum provides a JSON Schema file (`data-schema.json`) for IDE autocomplete, validation, and documentation. To use it:

**VS Code:**

Add this to the top of your `.data.yaml` file:
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/jprybylski/datum/main/data-schema.json
```

Or configure it globally in VS Code settings:
```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/jprybylski/datum/main/data-schema.json": ".data.yaml"
  }
}
```

**JetBrains IDEs (IntelliJ, PyCharm, etc.):**

1. Go to Settings → Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings
2. Add a new mapping:
   - Name: "Datum Configuration"
   - Schema URL: `https://raw.githubusercontent.com/jprybylski/datum/main/data-schema.json`
   - File path pattern: `.data.yaml`

**Local Schema:**

For offline use, reference the schema file locally:
```yaml
# yaml-language-server: $schema=./data-schema.json
```

The schema provides:
- Autocomplete for all fields and values
- Validation of data types and required fields
- Documentation on hover
- Handler-specific field validation based on `type`

### Configuration File Structure

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

### Multi-Source Configuration

Datum supports specifying multiple sources with automatic fallback. If the first source fails, datum will try subsequent sources in order:

```yaml
datasets:
  - id: my_data
    desc: Data with fallback sources
    sources:                  # Note: "sources" (plural) instead of "source"
      - type: http            # Primary source
        url: https://primary.example.com/data.csv
      - type: http            # Backup source (used if primary fails)
        url: https://backup.example.com/data.csv
      - type: file            # Local fallback
        path: ./cache/data.csv
    target: data/my_data.csv
```

**Key points:**
- Use either `source:` (single) or `sources:` (multiple), but not both
- Sources are tried in the order they are listed
- The final policy judgment is applied after all sources have been attempted
- Useful for high availability, geographic redundancy, and offline development

See the [Multi-Source Example](examples/multi-source/) for more details.

### Policy Options

- **`fail`**: Verification fails if the remote data has changed (strict mode)
- **`update`**: Automatically fetch and update if the remote data has changed
- **`log`**: Log changes but don't fail or update (monitoring mode)

## Commands

### `datum check`

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

### `datum fetch`

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

## Data Source Handlers

Datum uses a plugin-based handler system. Each handler knows how to fetch data from a specific source type.

### HTTP Handler (built-in)

Fetches data from HTTP/HTTPS URLs.

```yaml
source:
  type: http
  url: https://example.com/data.json
```

**Fingerprinting strategy:**
1. Try HTTP HEAD request for ETag header (most efficient)
2. Fall back to Last-Modified + Content-Length headers
3. Fall back to SHA256 hash of content (downloads file)

### File Handler (built-in)

Copies local files.

```yaml
source:
  type: file
  path: /absolute/path/to/source.txt
```

**Fingerprinting:** SHA256 hash of the file contents.

**Use cases:**
- Copying files from network shares
- Normalizing file locations in your project
- Tracking files on mounted volumes

### Command Handler (built-in)

Executes shell commands to fetch data.

```yaml
source:
  type: command
  fingerprint_cmd: "curl -sI https://example.com/data.csv | grep -i etag"
  fetch_cmd: "curl -o {{dest}} https://example.com/data.csv"
```

**Template variables:**
- `{{url}}` - source.url value
- `{{path}}` - source.path value
- `{{ref}}` - source.ref value
- `{{dest}}` - target file path

**Note:** The `DEST` environment variable is also set during fetch.

**Shell behavior:**
- **Linux/Mac**: Uses `/bin/sh`
- **Windows**: Uses PowerShell

### Git Handler (optional, requires `-tags git`)

Fetches specific files from git repositories.

```yaml
source:
  type: git
  url: https://github.com/owner/repo.git
  ref: main              # Branch or tag name
  path: LICENSE          # Path to file within the repository
```

**Fingerprinting:** Git blob SHA1 hash (native git object hash).

**Features:**
- Caches repositories in `~/.cache/datum/git/` (or `$XDG_CACHE_HOME`)
- Supports HTTPS and SSH authentication
- Shallow clones for efficiency
- Resolves branches and tags

**Authentication:**

For HTTPS:
```bash
export GIT_USERNAME=your-username
export GIT_PASSWORD=your-password
# or
export GIT_TOKEN=your-personal-access-token
```

For SSH:
```bash
# Uses SSH agent by default, or:
export GIT_SSH_KEY=/path/to/private/key
export GIT_SSH_PASSPHRASE=optional-passphrase
```

## Architecture and Implementation

The codebase demonstrates several important Go patterns and concepts:

### 1. Module System (`go.mod`)

The `go.mod` file defines this as a Go module with the path `github.com/jprybylski/datum`. This enables:
- Import paths like `github.com/jprybylski/datum/internal/core`
- Dependency management with versioning
- Reproducible builds

### 2. Package Structure

Go organizes code into packages. This project uses:

- **`cmd/datum/`** - Main application (package `main`)
- **`internal/`** - Internal packages (not importable by other projects)
  - **`internal/core/`** - Core business logic
  - **`internal/handlers/`** - Data source handlers
  - **`internal/registry/`** - Handler registration system
  - **`internal/runtime/`** - Platform-specific code

### 3. Interfaces

The handler system uses Go interfaces for polymorphism:

```go
type Fetcher interface {
    Name() string
    Fingerprint(ctx context.Context, src Source) (string, error)
    Fetch(ctx context.Context, src Source, dest string) error
}
```

Any type that implements these methods can be used as a handler.

### 4. Init Functions

Handlers self-register using `init()` functions:

```go
func init() {
    registry.Register(New())
}
```

Init functions run automatically when the package is imported, enabling plugin-like behavior.

### 5. Build Tags

The git handler uses build tags for conditional compilation:

```go
//go:build git
```

This file only compiles when you use `-tags git`, making git support optional.

### 6. Context Package

Functions use `context.Context` for:
- Cancellation signals
- Timeouts
- Request-scoped values

### 7. Error Handling

Go uses explicit error returns:

```go
func DoSomething() error {
    if err := operation(); err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }
    return nil
}
```

The `%w` verb wraps errors, preserving the error chain.

### 8. File Operations

The codebase demonstrates:
- Reading/writing YAML files
- Atomic file writes (write to `.tmp`, then rename)
- Creating directories with `os.MkdirAll`
- Hashing files with SHA256

## Project Structure

```
datum/
├── cmd/
│   └── datum/              # Main application entry point
│       ├── main.go         # CLI logic and command parsing
│       └── handlers_git.go # Git handler import (build tag)
│
├── internal/               # Internal packages
│   ├── core/              # Core business logic
│   │   ├── config.go      # Configuration file parsing
│   │   ├── engine.go      # Check and Fetch implementations
│   │   ├── hash.go        # File hashing utilities
│   │   └── lock.go        # Lockfile operations
│   │
│   ├── handlers/          # Data source handlers (plugins)
│   │   ├── http/
│   │   ├── file/
│   │   ├── git/          # Optional, requires build tag
│   │   └── command/
│   │
│   ├── registry/          # Handler registry system
│   │   └── registry.go
│   │
│   └── runtime/           # Platform-specific code
│       ├── shell_unix.go    # Unix/Linux shell execution
│       └── shell_windows.go # Windows shell execution
│
├── examples/              # Example configurations
│   ├── basic/
│   └── git-one-file/
│
├── scripts/               # Build scripts
│   ├── make.sh           # Linux/Mac build script
│   └── make.ps1          # Windows build script
│
├── go.mod                # Go module definition
├── go.sum                # Dependency checksums
└── README.md             # This file
```

### Key Files Explained

**`cmd/datum/main.go`** - Application entry point
- Parses command-line flags
- Dispatches to `core.Check()` or `core.Fetch()`
- Handles exit codes

**`internal/core/engine.go`** - Main logic
- `Check()`: Verifies datasets and applies policies
- `Fetch()`: Downloads datasets and updates lockfile

**`internal/registry/registry.go`** - Handler registry
- Global map of handler name -> implementation
- `Register()` and `Get()` functions

**`internal/handlers/*/` - Handler implementations
- Each handler implements the `Fetcher` interface
- Self-registers in `init()` function

## Development Guide

### Adding a New Handler

1. Create a new directory in `internal/handlers/`
2. Implement the `Fetcher` interface:

```go
package myhandler

import (
    "context"
    "github.com/jprybylski/datum/internal/registry"
)

type handler struct{}

func New() *handler { return &handler{} }

func (h *handler) Name() string { return "myhandler" }

func (h *handler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
    // Return a stable fingerprint for the source
    return "fingerprint", nil
}

func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
    // Download/copy data to dest
    return nil
}

func init() {
    registry.Register(New())
}
```

3. Import it in `cmd/datum/main.go`:

```go
_ "github.com/jprybylski/datum/internal/handlers/myhandler"
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./internal/core
```

### Code Quality

```bash
# Run the linter (if configured)
golangci-lint run

# Run go vet (static analysis)
go vet ./...

# Format code
go fmt ./...
```

### Building for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/datum-linux ./cmd/datum

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/datum.exe ./cmd/datum

# macOS
GOOS=darwin GOARCH=amd64 go build -o bin/datum-mac ./cmd/datum
```

## Examples

Complete working examples are available in the `examples/` directory.

### Example 1: HTTP Handler - Tracking CDC Growth Chart Data

From [`examples/basic/.data.yaml`](examples/basic/.data.yaml):

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

This example fetches CDC reference data for weight-for-age charts. The `fail` policy ensures your pipeline breaks if the upstream data changes unexpectedly.

**Try it:**
```bash
cd examples/basic
datum --config .data.yaml fetch
datum --config .data.yaml check
```

### Example 2: Git Handler - Tracking Dependency Licenses

From [`examples/git-one-file/.data.yaml`](examples/git-one-file/.data.yaml):

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

This example tracks the LICENSE file from a GitHub repository, useful for compliance tracking or ensuring you're always using the correct license text.

**Try it:**
```bash
cd examples/git-one-file
datum --config .data.yaml fetch
datum --config .data.yaml check
```

### Example 3: File Handler - Copying Local Files

From [`examples/file-copy/.data.yaml`](examples/file-copy/.data.yaml):

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

Use the file handler to copy files from local paths or network shares, with automatic updates when the source changes.

**Try it:**
```bash
cd examples/file-copy
datum --config .data.yaml fetch
datum --config .data.yaml check
```

### Example 4: Command Handler - System Information

From [`examples/command-system/.data.yaml`](examples/command-system/.data.yaml):

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

The command handler allows custom fetch logic using shell commands. This example captures system information and uses a date-based fingerprint.

**Try it:**
```bash
cd examples/command-system
datum --config .data.yaml fetch
datum --config .data.yaml check
```

### Example 5: Multi-Source with Fallback

From [`examples/multi-source/.data.yaml`](examples/multi-source/.data.yaml):

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

This example demonstrates multi-source functionality where datum automatically falls back to alternative sources if the primary source fails. This is useful for high availability, geographic redundancy, and offline development workflows.

**Try it:**
```bash
cd examples/multi-source
datum --config .data.yaml fetch
datum --config .data.yaml check
```

### Example 6: Multiple Datasets with Different Policies

From [`examples/multi-policy/.data.yaml`](examples/multi-policy/.data.yaml):

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

This example demonstrates using different policies for different types of data: strict verification for critical data, automatic updates for documentation, and monitoring-only for informational tracking.

**Try it:**
```bash
cd examples/multi-policy
datum --config .data.yaml fetch
datum --config .data.yaml check
```

## FAQ

### Why "datum"?

Datum is the singular form of "data" - fitting for a tool that manages individual data sources.

### How is this different from downloading files manually?

Datum provides:
- Automated verification
- Cryptographic fingerprints
- Policy-based handling of changes
- Single configuration file for all data sources
- Reproducibility for your entire data pipeline

### Can I use this in CI/CD?

Yes! Use `datum check` in your CI pipeline to verify that external data hasn't changed unexpectedly.

### What happens if my policy is "fail" and data changes?

The `check` command will exit with code 1, and you'll see which datasets have changed. You can then:
1. Investigate why the data changed
2. Run `datum fetch <dataset-id>` to update specific datasets
3. Commit the updated lockfile

### How do I version control the lockfile?

Yes, commit both `.data.yaml` and `.data.lock.yaml` to version control. This ensures:
- Team members have the same data versions
- Historical record of when data changed
- Reproducible builds

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

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our development process, coding standards, and how to submit pull requests.
