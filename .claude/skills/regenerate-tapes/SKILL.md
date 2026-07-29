---
name: regenerate-tapes
description: Rebuild datum and re-run VHS over docs/assets/tapes/*.tape to refresh the demo GIFs in docs/assets/img/ after a CLI output change.
---

# Regenerate demo tape GIFs

Every file in `docs/assets/tapes/*.tape` records a live `datum` terminal
session and renders it to the matching GIF in `docs/assets/img/`. Each tape's
header comment says "regenerate this whenever the CLI's output format
changes" - this skill does that regeneration.

Run this whenever a change touches anything a tape's terminal output would
show: `cmd/datum/**`, `internal/core/**` (progress lines, colors, JSON
output), or `internal/handlers/**` (handler-specific output/behavior).

## Steps

1. **Confirm `vhs` is installed** (`vhs --version`). If missing, stop and
   tell the user to install it (e.g. `brew install vhs`) - don't try to
   install it yourself.

2. **Build both `datum` variants** into a scratch bin directory so both
   git-tagged and non-git tapes can find a binary on `PATH`:
   ```bash
   mkdir -p /tmp/datum-tape-bin
   go build -tags git -o /tmp/datum-tape-bin/datum ./cmd/datum
   ```
   (`-tags git` is a superset - it's fine for the non-git tapes too, so one
   build covers all 7.)

3. **Run every tape from the repo root**, with that bin directory first on
   `PATH` (some tapes' setup scripts reference repo-relative paths like
   `docs/assets/tapes/*-setup.sh`, so the working directory matters):
   ```bash
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/basic-fetch.tape
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/command-system.tape
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/directory-sync.tape
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/file-copy.tape
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/git-one-file.tape
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/multi-source.tape
   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/policy-reactions.tape
   ```
   `basic-fetch.tape` and `git-one-file.tape` hit real network endpoints
   (a CDC data file and a GitHub repo, respectively) - if there's no network
   access available, note which tapes couldn't be re-rendered rather than
   silently skipping them.

4. **Report what changed**, not just "done":
   ```bash
   git status --short docs/assets/img/
   ```
   GIFs are binary, so there's no useful `git diff` - list which of the 7
   changed and which didn't. An unchanged GIF for a tape whose underlying
   output *did* change is worth flagging back to the user, since it usually
   means that particular tape doesn't exercise the changed code path.

5. Leave the refreshed GIFs staged/modified in the working tree for the user
   (or the calling agent) to review and commit - don't commit automatically.
