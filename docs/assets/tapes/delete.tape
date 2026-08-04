# Records a live `datum delete` / `datum undelete` round trip against the
# "file-copy" example (examples/file-copy - a single local file tracked with
# the File handler): fetch, delete (with --yes to skip the interactive
# prompt), a check showing the deleted dataset being skipped rather than
# treated as missing/failed, then undelete + fetch to restore it (#17).
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/delete.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/delete.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

# Work from a throwaway copy of the example so the recording always starts
# from a clean slate (no pre-existing lockfile or copied-config directory).
Hide
Type `SRC="$PWD"; cd "$(mktemp -d)"; cp "$SRC/examples/file-copy/.data.yaml" .; cp "$SRC/examples/file-copy/source-config.json" .; clear`
Enter
Sleep 500ms
Show

Type "datum fetch"
Enter
Sleep 2s

Type "datum --yes delete local_config"
Enter
Sleep 2.5s

Type "ls config/ 2>&1"
Enter
Sleep 1.5s

Type "datum check"
Enter
Sleep 2.5s

Type "datum undelete local_config"
Enter
Sleep 2s

Type "datum fetch local_config"
Enter
Sleep 2.5s
