# Records a live `datum fetch` + `datum check` run against the "basic" example
# (examples/basic - a single HTTP dataset).
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/basic-fetch.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/basic-fetch.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 650
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

# Work from a throwaway copy of the example config so the recording always
# starts from a clean slate (no pre-existing lockfile or downloaded data).
Hide
Type `SRC="$PWD"; cd "$(mktemp -d)"; cp "$SRC/examples/basic/.data.yaml" .; clear`
Enter
Sleep 500ms
Show

Type "cat .data.yaml"
Enter
Sleep 2s

Type "ls -la"
Enter
Sleep 1.5s

Type "datum fetch"
Enter
Sleep 3s

Type "ls -la data/ref/"
Enter
Sleep 1.5s

Type "datum check"
Enter
Sleep 3s
