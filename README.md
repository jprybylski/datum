# pinup (project skeleton)

A small tool to pin external data sources (HTTP, file, git via build tag, custom commands) to a lockfile with fingerprints.

## Build

```bash
go mod tidy
go build ./cmd/pinup
# include git handler:
go build -tags git ./cmd/pinup
```

## Run

```bash
./pinup --config examples/basic/.data.yaml check
./pinup --config examples/basic/.data.yaml fetch
```

## Config

See `examples/*/.data.yaml`.

## Handlers

- `http` (built-in)
- `file` (built-in)
- `command` (built-in; runs shell commands; PowerShell on Windows, sh on Unix)
- `git` (optional; build with `-tags git`)
