---
layout: default
title: Embedding datum
nav_order: 6.5
---

# Embedding datum

Run on your own project, datum trusts its `.data.yaml`. A config can run shell commands, write
to any path, and read any environment variable. That's fine for a config you wrote, but not when
datum runs on a config someone else wrote. Examples:

- a desktop or web app that runs datum for its users;
- a CI job over shared project folders;
- a wrapper library;
- a downloaded template.

For those cases datum has opt-in global flags. Each one turns a check the caller would otherwise
do itself into a refusal by datum, before any source is contacted. Without them, datum behaves
exactly as before.

Like every global flag, these go **before** the command: `datum --no-vcs --root . check`.

## The flags

| Flag | What it guarantees |
|---|---|
| `--no-vcs` (or `DATUM_NO_VCS=1`) | datum never starts `git` or `svn` and never edits `.gitignore` or `svn:ignore`. A dataset with `ignore: true` gets a warning instead. |
| `--allow-types http,file,git` (or `DATUM_ALLOW_TYPES`) | A config that uses any other source type, in `source` or any `sources` entry, is refused at load (exit 2). Use it to rule out `command`. |
| `--root DIR` | Targets and relative `file` sources resolve in `DIR`, not the working directory. Every target must be relative, contain no `..`, and stay inside `DIR` after following symlinks. A target can't be the config or the lockfile. |
| `--target-prefix PATH` | Every target must lie under `PATH`, relative to the root. |
| `--env-allow A,B` / `--env-prefix P` | A `${NAME}` reference to any other variable is refused at load, naming the dataset and field. |
| `--https-only` | `http` and `git` sources must use `https://`. |
| `--git-https-only` | `git` sources must use `https://`. No SSH agent or key is ever used. |
| `--cache-dir DIR` | The git clone cache goes under `DIR` instead of `XDG_CACHE_HOME` or `~/.cache/datum`. |
| `--report-only` (check) | Nothing is written: no targets, no lockfile, no ignore rules. `update` datasets that would be re-fetched are reported as `stale`, with the message "would update". |
| `--stage-dir DIR --lock-out PATH` (fetch) | Fetched targets are written under `DIR` and the new lockfile to `PATH`. The real targets, lockfile and ignore rules are untouched. See below. |
| `--verify-local` (audit) | Each tracked target is hashed and compared with the lockfile. See [audit drift](#audit-drift). |

A refusal is a config error: exit code 2, and with `--json` a single `{"error": ...}` document.
The message names the dataset (`dataset 2 (portal): source.url: ...`) and never includes a
substituted value.

## Always on

These need no flag:

- **Scoped substitution.**
  - `fetch ID ...` substitutes `${NAME}` only in the datasets it fetches, so it needs only
    their variables.
  - `audit`, `undelete` and `unlock` need no variables: an unset reference is shown as
    written.
  - `delete ID ...` needs only the variables in its datasets' targets.
  - `check` still needs every variable, because it reads every source.
- **Redaction.** Before datum prints or stores a message, it replaces with `[redacted]`:
  - every value it substituted for `${NAME}` (values shorter than 4 characters are left
    alone);
  - the `user:password@` part of any URL;
  - every URL query value.

  This covers text, `--json`, and `inaccessible_error` in the lockfile.
- **HTTP redirects.**
  - A redirect from `https://` to `http://` is refused.
  - A redirect to another host drops `Authorization` and every header from the source's
    `headers`.
  - At most 10 redirects are followed.
- **Lockfile paths.** `dir_paths` and `dir_files` entries must be relative paths without `..`.
  Otherwise the lockfile is refused, so a hand-edited lockfile can't make datum delete files
  outside a target.

## A typical invocation

```bash
datum --json --no-color --no-vcs \
  --allow-types http,file,git --https-only --git-https-only \
  --root "$PROJECT" --target-prefix source-data \
  --env-allow PORTAL_TOKEN \
  --config "$PROJECT/.data.yaml" --lock "$PROJECT/.data.lock.yaml" \
  --report-only check
```

`--report-only` is a global flag too, so it comes before `check`.

## Staged fetch and promote

A staged fetch lets a caller show the user what a fetch would change, then apply it as a whole:

```bash
datum --json --no-vcs --root "$PROJECT" \
  --stage-dir "$STAGE" --lock-out "$STAGE/lock.yaml" fetch portal share
```

- Each target is written to `$STAGE/<target>`. Targets must be relative.
- The new lockfile goes to `--lock-out`. It's identical to the one an in-place fetch would have
  written (apart from timestamps), including directory hashes when the target folder is shared
  with other datasets or files.
- Each result in the `--json` output has a `staged` object:

```json
{
  "id": "share",
  "status": "fetched",
  "remote_fingerprint": "dirsha256:…",
  "staged": {
    "target": "source-data/share",
    "path": "/stage/source-data/share",
    "kind": "dir",
    "sha256": "…",
    "files": [{"path": "a.csv", "sha256": "…", "size": 1204}],
    "removed": ["old.csv"]
  }
}
```

For a single-file target, `kind` is `"file"` and `sha256` and `size` describe it. For a
directory, `files` lists every file written and `removed` lists the files the previous fetch
wrote that this one no longer has.

To **promote** a staged fetch, the caller:

1. checks each staged file's SHA-256 against `staged.files` (or `staged.sha256`);
2. optionally keeps a copy of each file it will replace;
3. moves each staged file into place with an atomic rename on the same filesystem;
4. deletes each path in `removed` from the target;
5. replaces the real lockfile with `--lock-out` (write to a temporary file, then rename).

If the caller is interrupted, it can restart from step 3: every step can be repeated safely.
Datasets with `status` other than `fetched` have no `staged` object and must not be promoted.

## Directory sync

An in-place directory fetch replaces the target as a whole when it holds only that dataset's
files. The new tree is built next to it (`<target>.datum-new-*`), the old one is moved aside
(`<target>.datum-old-*`), and the new one takes its place. If datum stops between those two
renames, the next `check` or `fetch` restores the old tree.

When the folder also holds other files or another dataset's files, datum writes every new file
first, then renames them all into place, then removes files that are gone upstream.

## Audit drift

`datum --verify-local audit` adds `drift` to each tracked entry:

| Value | Meaning |
|---|---|
| `ok` | The local target matches the lockfile. |
| `changed` | It doesn't. |
| `missing` | The target is gone. |
| `unknown` | The lockfile has no hash to compare with. |

For directory targets fetched by this version, `files` lists each recorded file with its own
`drift`. The audit `status` values are unchanged.

## JSON format

- **The `format` field.** Every JSON document except `datum schema` has `"format":
  "datum-report/1"`. The format changes only on an incompatible change; new optional fields
  don't change it, so consumers should ignore fields they don't know.
- **Version.** `datum --json --version` prints `{"format", "version", "features"}`, where
  `features` lists optional build features such as `"git"`. Plain `datum --version` still
  prints one line, `datum vX.Y.Z`.
- **`delete`, `undelete`, `unlock`.** With `--json` these print `{"format", "results": [{"id",
  "status", "message"}]}`, where `status` is one of `deleted`, `undeleted`, `unlocked`,
  `skipped` or `error`. With `--json`, `delete` and `unlock` need `--yes`, since there's no
  prompt.
- **`init`.** With `--json` it never prompts and prints `{"format", "config", "datasets"}`.
- **Exit codes** are the same with or without `--json`.

## Lockfile additions

The lockfile stays at version 1. A fetch now also records:

- `size` for a single-file target;
- `dir_files: [{path, sha256, size}]` for a directory, next to `dir_paths`.

Older datum versions ignore these fields, and older lockfiles still load.

## Release assets

Released archives never change: a different build always gets a new version. The binaries are
static (`CGO_ENABLED=0`) and need no Git, shell or runtime libraries. So you can pin a release
archive by the SHA-256 in its `checksums.txt`.
