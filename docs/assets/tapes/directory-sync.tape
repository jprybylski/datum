# Records a live `datum fetch` against the "directory-sync" example
# (examples/directory-sync - a File handler pointed at a directory), then
# deletes a file from the source and re-fetches to show deletion tracking:
# files removed from the source directory get removed from the target too.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/directory-sync.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/directory-sync.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 750
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

# Work from a throwaway copy of the example so the recording always starts
# from a clean slate (no pre-existing lockfile or synced-data/ directory).
Hide
Type `SRC="$PWD"; cd "$(mktemp -d)"; cp "$SRC/examples/directory-sync/.data.yaml" .; cp -r "$SRC/examples/directory-sync/source-data" .; clear`
Enter
Sleep 500ms
Show

Type "find source-data -type f | sort"
Enter
Sleep 2s

Type "datum fetch"
Enter
Sleep 2.5s

Type "find synced-data -type f | sort"
Enter
Sleep 2s

Type "rm source-data/nested/note.txt"
Enter
Sleep 1.5s

Type "datum fetch"
Enter
Sleep 2.5s

Type "find synced-data -type f | sort"
Enter
Sleep 3s
