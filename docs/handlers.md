---
layout: default
title: Handlers
nav_order: 5
---

# Data Source Handlers

Datum uses a plugin-based handler system. Each handler knows how to fetch data from a specific
source type.

## HTTP Handler (built-in)

Fetches data from HTTP/HTTPS URLs.

```yaml
source:
  type: http
  url: https://example.com/data.json
  headers:
    Authorization: "Bearer ${API_TOKEN}"
    Content-Type: application/json
  body: '{"format":"json"}'
```

`headers` is an optional string-to-string map and is applied to every request, including the HEAD
probe used for ordinary GET sources. A non-empty `body` changes the request to POST; without one,
the existing HEAD/GET behavior is unchanged. Header and body values support normal `${NAME}`
environment substitution, so credentials do not need to be committed.

POST endpoints must be safe to repeat: both `check` and `fetch` execute the configured request.
A fetch derives the fingerprint from the same response it writes, so it does not issue a second
POST merely to fingerprint the result.

**Fingerprinting strategy for ordinary GET sources:**
1. Try HTTP HEAD request for ETag header (most efficient)
2. Fall back to Last-Modified + Content-Length headers
3. Fall back to SHA256 hash of content (downloads file)

POST sources use the same metadata preference on the POST response itself, then hash that response
when no useful metadata is present.

## File Handler (built-in)

Copies local files - or, if `path` points at a directory, recreates the entire directory tree
under `target`.

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

### Directory sources

If `source.path` is a directory (auto-detected at run time - no separate config needed), `target`
is treated as a directory too:

```yaml
source:
  type: file
  path: /mnt/shared/dataset/
target: data/dataset/
```

- Every file under `path` is copied into `target`, preserving relative structure. `target`
  doesn't need to share the source directory's name or be otherwise empty first.
- The fingerprint (`dirsha256:...`) is an aggregate hash over every file's relative path and
  content, so any addition, removal, rename, or edit anywhere in the tree changes it.
- If a file disappears from the source between fetches, it's removed from `target` too - but
  only files this dataset previously wrote. Anything else already living in `target` is left
  alone, since `target` isn't assumed to hold only this dataset's contents. datum tracks which
  relative paths each dataset wrote in the lockfile (`dir_paths` on that dataset's entry) rather
  than a sidecar file next to `target` - don't hand-edit that field, or the next fetch won't know
  what it's allowed to clean up.
- More than one dataset can target the same directory - useful for merging several sources into
  one place. They can even have same-named subdirectories, as long as the datasets don't write
  the same relative path within `target`; if two datasets do claim the same relative path, the
  second one to fetch fails with an error instead of silently overwriting the first one's file.
  Sharing a target this way is only safe when those datasets are fetched at `--concurrency 1`
  (the default) - conflict detection isn't guaranteed to catch the case where two datasets
  sharing a target are fetched fully in parallel.
- Directory sync isn't a single atomic operation; files are copied/removed one at a time. A
  crash mid-fetch can leave `target` partially updated - re-running `fetch` finishes the job.

See it in action: [Example 7 - Directory Sync]({{ '/examples.html' | relative_url }}#example-7-file-handler---directory-sync).

## Command Handler (built-in)

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

**Note:** The `DEST` environment variable is also set during fetch, alongside your normal
inherited environment (`PATH`, `HOME`, etc.) - so commands can freely shell out to real binaries,
not just shell builtins.

**Shell behavior:**
- **Linux/Mac**: Uses `/bin/sh`
- **Windows**: Uses `cmd.exe` (not PowerShell, to avoid its UTF-16 default encoding for file
  redirects)

## Git Handler (optional, requires `-tags git`)

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
- Resolves both branches and tags

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

SSH host keys are verified against `SSH_KNOWN_HOSTS` (or `~/.ssh/known_hosts` /
`/etc/ssh/ssh_known_hosts`) by default, same as the `ssh` CLI. If a host isn't in your
known_hosts file, the fetch will fail rather than silently trusting it. For CI environments or
throwaway containers where that verification isn't practical, you can explicitly disable it:
```bash
export DATUM_GIT_INSECURE_HOST_KEY=1  # skips SSH host-key verification - MITM risk, use with care
```

---

Next: [Examples]({{ '/examples.html' | relative_url }}) for complete, runnable configs using each of these
handlers.
