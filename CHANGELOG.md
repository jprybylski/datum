# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.1] - 2026-07-28

### Added
- Colorized `check`/`fetch` output: status tags (`[OK  ]` green, `[FAIL]`/`[ERR ]` red,
  `[WARN]`/`[STALE]` yellow, `[UPD ]`/`[FETCH]` cyan) are colored when stdout is a terminal, and
  automatically plain otherwise - piped output, `NO_COLOR` (per [no-color.org](https://no-color.org/)),
  and the new `--no-color` flag all suppress it.
- `--json` flag for `check`/`fetch`: prints a single JSON document (`{"results": [...]}`, one
  object per dataset with `id`/`status`/fingerprints/`message`/`warnings`) instead of colorized
  text, for scripting and other programmatic consumers. Config/lockfile-level errors print as
  `{"error": "..."}` under `--json` instead of plain text. Exit codes are unchanged either way.

### Changed
- `[STALE]`/`[FAIL]` lines showing a changed fingerprint now print the old (dimmed) and new
  fingerprint on their own indented lines instead of a single `(lock="..." -> now="...")` line -
  fingerprints are frequently full sha256 hashes or ETags 60+ characters long, and quoting them
  with `%q` made them read like an escaped Go literal rather than a value to compare. A dataset
  with no prior lock entry now shows `lock: (none)` instead of the internal `lock="<nil>"`.
- Test coverage raised from 85.5% to 96.7% overall, closing most of the error-handling branches
  Codecov flagged as untested (all packages now above 90%).

### Fixed
- `check`/`fetch` no longer panic when the lockfile exists but contains invalid YAML (as opposed
  to not existing, which was already handled gracefully) - they now exit `2` with a clear "lock
  error" message, same as a config parse error.

## [1.2.0] - 2026-07-28

### Added
- Directory sources for the `file` handler: pointing `source.path` at a directory (auto-detected)
  recreates the whole tree under `target`, with an aggregate `dirsha256:` fingerprint and
  deletion tracking so files removed upstream are removed from `target` too, without touching
  anything else already living there (closes #8).
- `--timeout` and `--concurrency` flags for both `check` and `fetch`. `--timeout` (default `5m`)
  bounds the entire run via `context.Context` so a hung source can't block the process forever;
  `--concurrency` (default `1`) processes datasets in parallel while keeping output in the
  original config-file order regardless of completion order.
- `--version` flag, printing the build's version (set via `-ldflags -X main.version=...` in
  release builds; local `go build` binaries report `dev`).
- Docker-based integration test suite (`test/integration/`, run via
  `scripts/test-integration.sh`) exercising the git handler against a real git-over-SSH server -
  covering the real SSH transport and host-key verification behavior that in-process unit tests
  can't reach. `scripts/sandbox.sh` brings up the same server as a persistent, manually
  resettable sandbox (`up` / `down` / `reset` / `status`) for trying `datum` against it by hand,
  with a ready-made `test/integration/sandbox.data.yaml`.
- Documentation site under `docs/`, published via GitHub Pages
  (`.github/workflows/pages.yml`) - installation, configuration, commands, handlers, examples,
  tool comparison, and architecture/development pages with real navigation and search. The
  README is now a shorter landing page linking out to it instead of holding everything inline.
- Release automation: pushing a `VERSION` bump to `main` tags and publishes a GitHub Release via
  goreleaser (`.goreleaser.yml`, `.github/workflows/tag-release.yml` +
  `.github/workflows/release.yml`), with cross-compiled binaries for Linux/macOS/Windows
  (amd64+arm64), checksums, and this changelog's matching section as release notes.

### Fixed
- **Security:** the git handler's SSH auth no longer unconditionally disables host-key
  verification. It now uses go-git's secure default (verifying against `known_hosts`) unless
  `DATUM_GIT_INSECURE_HOST_KEY=1` is explicitly set.
- **Correctness:** git tag refs (`ref: v1.0.0`) were silently unresolvable due to dead fallback
  logic in `resolveRefCommit` - a bare ref was always pre-normalized to a branch ref before the
  tag-fallback check could ever run. Only branch refs were exercised by the existing example/CI
  coverage, so this went unnoticed.
- **Correctness:** the `command` handler's custom environment variables (`fetch_cmd`,
  `fingerprint_cmd`) were replacing the entire child process environment instead of extending it,
  silently wiping `PATH`/`HOME`/etc. and breaking any command that wasn't a shell builtin.
- Check's `update` policy no longer silently keeps a stale fingerprint when a fetch succeeds but
  the immediate post-fetch re-fingerprint fails; it's now treated as a failed attempt (matching
  `fetch`'s existing behavior), so the next source is tried instead.
- `writeLock` now creates its lockfile's parent directory if it doesn't exist yet, matching every
  handler's own file-write path - a fresh `--lock` path pointing into a not-yet-created directory
  used to fail with a confusing "no such file or directory".
- **Security:** resolved all 27 open Dependabot alerts (13 critical/high) by updating
  `github.com/go-git/go-git/v5` (v5.13.0 → v5.19.1), `golang.org/x/crypto` (v0.36.0 → v0.54.0),
  and `golang.org/x/net` (v0.38.0 → v0.57.0), which pulled in patched transitive dependencies
  (`go-billy`, `circl`) as well. Verified against the full test suite, both build tag variants,
  and the Docker-based SSH integration tests. **This raises the minimum Go version to 1.25**
  (go-git v5.19.1's own `go.mod` requires it) - CI, README, and CONTRIBUTING.md updated
  accordingly.

### Changed
- Test coverage: `cmd/datum`, the `git` handler, and `internal/runtime` were previously untested
  (0%); all packages now have meaningful coverage (76-100%).
- `errcheck` re-enabled in the linter config (previously disabled project-wide).
- Deduplicated the source-fallback loop shared by `Check`/`Fetch` in `internal/core/engine.go`.

## [1.0.0] - 2025-01-02

### Added

**Core Features:**
- Data pinning and verification system with cryptographic fingerprints
- `fetch` command to download data and record fingerprints
- `check` command to verify data sources haven't changed
- Lockfile system (`.data.lock.yaml`) for tracking fingerprints and verification timestamps
- Three policy modes: `fail` (strict), `update` (auto-update), and `log` (monitoring)

**Data Source Handlers:**
- HTTP/HTTPS handler with smart fingerprinting (ETag → Last-Modified+Content-Length → SHA256)
- File handler for copying local files
- Git handler (optional, requires `-tags git`) for tracking specific files from repositories
- Command handler for custom shell commands with template variable support
- Plugin-based handler architecture for extensibility

**Multi-Source Support:**
- Automatic fallback between multiple sources for high availability
- Sources tried in order until one succeeds
- Useful for mirrors, geographic redundancy, and offline development

**Build and Platform Support:**
- Go 1.23+ support
- Cross-platform support (Linux, macOS, Windows)
- Platform-specific shell handling (sh for Unix, cmd.exe for Windows)
- Optional git support via build tags to reduce binary size
- Build scripts for Unix (`make.sh`) and Windows (`make.ps1`)

**Developer Experience:**
- JSON Schema (`data-schema.json`) for IDE autocomplete and validation
- Comprehensive documentation and examples
- Six working examples covering all handler types
- Educational code comments explaining Go concepts

**CI/CD Integration:**
- GitHub Actions workflow with test, build, lint, and examples jobs
- Testing on Ubuntu, macOS, and Windows with Go 1.23 and stable
- golangci-lint integration
- Code coverage tracking

**Documentation:**
- Comprehensive README with architecture explanations
- Comparison with other tools (DVC, pins, Pooch, Quilt, LakeFS, Pachyderm)
- Contributing guidelines (CONTRIBUTING.md)
- MIT License
- AI development acknowledgment

### Technical Details

**Package Structure:**
- `cmd/datum/` - CLI entry point
- `internal/core/` - Core business logic (config, engine, lock, hash)
- `internal/handlers/` - Pluggable handler implementations
- `internal/registry/` - Handler registration system
- `internal/runtime/` - Platform-specific code

**Key Patterns:**
- Interface-based handler system for polymorphism
- Init function registration for plugin architecture
- Context-based cancellation and timeout support
- Atomic file operations (temp file + rename)
- Error wrapping with `%w` for error chains

**Security:**
- SHA256 fingerprinting for data integrity
- Git authentication support (HTTPS tokens, SSH keys)
- No credential storage in configuration files

[1.2.0]: https://github.com/jprybylski/datum/releases/tag/v1.2.0
[1.0.0]: https://github.com/jprybylski/datum/releases/tag/v1.0.0
