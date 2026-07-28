---
layout: default
title: Installation
nav_order: 2
---

# Installation

## Prebuilt Binaries

If you don't want to clone the repo or have Go installed, grab a binary from the
[latest release](https://github.com/jprybylski/datum/releases/latest) - Linux, macOS, and
Windows, each for amd64 and arm64, all built with git support included. Download the archive
for your platform, extract it, and run the `datum` binary inside.

With the [GitHub CLI](https://cli.github.com/) this can be one command, and always gets the
current release without you needing to know the version number:

```bash
gh release download --repo jprybylski/datum --pattern '*linux_amd64*'   # pick your platform/arch
tar -xzf datum_*_linux_amd64.tar.gz  # or unzip on Windows
./datum --version
```

Each release also includes a `checksums.txt` if you want to verify the download.

## Prerequisites (building from source)

- Go 1.25 or later
- Git (if you want git repository support)

## Build from Source

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

The binary will be in `./bin/datum` (or `./bin/datum.exe` on Windows).

## Build Scripts Explained

The project includes helper scripts in the `scripts/` directory:

- **`make.sh`** (Linux/Mac): Runs `go mod tidy`, `go vet`, and builds the binary
- **`make.ps1`** (Windows): Same as above, but for PowerShell

You can pass build tags as arguments:

```bash
# Build with git support
bash scripts/make.sh git
```

## Building for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/datum-linux ./cmd/datum

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/datum.exe ./cmd/datum

# macOS
GOOS=darwin GOARCH=amd64 go build -o bin/datum-mac ./cmd/datum
```

Next: [Configuration]({{ '/configuration.html' | relative_url }}) to write your first `.data.yaml`.
