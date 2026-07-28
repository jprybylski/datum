---
layout: default
title: Architecture & Development
nav_order: 8
---

# Architecture and Implementation

The codebase demonstrates several important Go patterns and concepts.

## Module System (`go.mod`)

The `go.mod` file defines this as a Go module with the path `github.com/jprybylski/datum`. This
enables:
- Import paths like `github.com/jprybylski/datum/internal/core`
- Dependency management with versioning
- Reproducible builds

## Package Structure

Go organizes code into packages. This project uses:

- **`cmd/datum/`** - Main application (package `main`)
- **`internal/`** - Internal packages (not importable by other projects)
  - **`internal/core/`** - Core business logic
  - **`internal/handlers/`** - Data source handlers
  - **`internal/registry/`** - Handler registration system
  - **`internal/runtime/`** - Platform-specific code

```
datum/
├── cmd/
│   └── datum/              # Main application entry point
│       ├── main.go         # CLI logic and command parsing
│       └── handlers_git.go # Git handler import (build tag)
│
├── internal/               # Internal packages
│   ├── core/               # Core business logic
│   │   ├── config.go       # Configuration file parsing
│   │   ├── engine.go       # Check and Fetch implementations
│   │   ├── hash.go         # File hashing utilities
│   │   └── lock.go         # Lockfile operations
│   │
│   ├── handlers/            # Data source handlers (plugins)
│   │   ├── http/
│   │   ├── file/
│   │   ├── git/            # Optional, requires build tag
│   │   └── command/
│   │
│   ├── registry/            # Handler registry system
│   │   └── registry.go
│   │
│   └── runtime/             # Platform-specific code
│       ├── shell_unix.go    # Unix/Linux shell execution
│       └── shell_windows.go # Windows shell execution
│
├── examples/                # Example configurations (see Examples page)
│
├── test/
│   └── integration/         # Docker-based integration tests (build tag: integration)
│       └── gitserver/       # Minimal git-over-SSH server used by those tests
│
├── scripts/                 # Build and test scripts
│   ├── make.sh               # Linux/Mac build script
│   ├── make.ps1               # Windows build script
│   └── test-integration.sh    # Runs the Docker-based integration suite
│
├── go.mod
├── go.sum
└── README.md
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

**`internal/handlers/*/`** - Handler implementations
- Each handler implements the `Fetcher` interface
- Self-registers in `init()` function

## Interfaces

The handler system uses Go interfaces for polymorphism:

```go
type Fetcher interface {
    Name() string
    Fingerprint(ctx context.Context, src Source) (string, error)
    Fetch(ctx context.Context, src Source, dest string) error
}
```

Any type that implements these methods can be used as a handler.

## Init Functions

Handlers self-register using `init()` functions:

```go
func init() {
    registry.Register(New())
}
```

Init functions run automatically when the package is imported, enabling plugin-like behavior.

## Build Tags

The git handler uses build tags for conditional compilation:

```go
//go:build git
```

This file only compiles when you use `-tags git`, making git support optional.

## Context Package

Functions use `context.Context` for:
- Cancellation signals
- Timeouts (see `--timeout` on the [Commands]({% link commands.md %}) page)
- Request-scoped values

## Error Handling

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

## File Operations

The codebase demonstrates:
- Reading/writing YAML files
- Atomic file writes (write to `.tmp`, then rename)
- Creating directories with `os.MkdirAll`
- Hashing files with SHA256

---

# Development Guide

## Adding a New Handler

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

## Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./internal/core
```

**Integration tests:** a separate suite under `test/integration/` (gated behind the
`integration` build tag, so it's excluded from `go test ./...`) exercises the git handler
against a real git-over-SSH server in Docker, covering what fully in-process unit tests can't:
the actual SSH transport and host-key verification behavior. Requires Docker:

```bash
bash scripts/test-integration.sh
```

This builds an ephemeral SSH keypair and git server container, runs the tests, and tears
everything down again - nothing persists between runs.

## Code Quality

```bash
# Run the linter
golangci-lint run

# Run go vet (static analysis)
go vet ./...

# Format code
go fmt ./...
```

---

Read [CONTRIBUTING.md](https://github.com/jprybylski/datum/blob/main/CONTRIBUTING.md) on GitHub
for the full contribution workflow.
