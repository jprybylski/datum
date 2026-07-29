# Records a live `datum fetch` + `datum check` run against the "file-copy"
# example (examples/file-copy - the File handler pointed at a single local
# file), then edits the source file to show the `update` policy silently
# re-pinning and re-copying on the next check.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/file-copy.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/file-copy.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 650
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

Type "cat source-config.json"
Enter
Sleep 2s

Type "datum fetch"
Enter
Sleep 2s

Type "cat config/copied.json"
Enter
Sleep 2s

Type `sed -i.bak 's/"debug": false/"debug": true/' source-config.json`
Enter
Sleep 1.5s

Type "datum check"
Enter
Sleep 2.5s

Type "cat config/copied.json"
Enter
Sleep 3s
