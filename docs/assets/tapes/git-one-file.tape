# Records a live `datum fetch` + `datum check` run against the "git-one-file"
# example (examples/git-one-file - the Git handler, requires a `-tags git`
# build), pulling a single file (LICENSE) out of a remote repo without a
# full clone.
#
# Run from the repo root, with a `-tags git` datum build on $PATH, e.g.:
#   go build -tags git -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/git-one-file.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/git-one-file.gif

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
Type `SRC="$PWD"; cd "$(mktemp -d)"; cp "$SRC/examples/git-one-file/.data.yaml" .; clear`
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
Sleep 4s

Type "ls -la data/ref/ && head -3 data/ref/google_uuid_LICENSE.txt"
Enter
Sleep 2.5s

Type "datum check"
Enter
Sleep 3s
