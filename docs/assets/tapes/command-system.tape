# Records a live `datum fetch` + `datum check` run against the
# "command-system" example (examples/command-system - the Command handler,
# fingerprinting with `date +%Y-%m-%d` and fetching with `uname -a`).
#
# Only the OS name and kernel version are printed from the fetched output
# (not the full `uname -a` line), so the recording doesn't bake this
# machine's hostname into a GIF that ships in the public docs.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/command-system.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/command-system.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 650
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

# Work from a throwaway copy of the example so the recording always starts
# from a clean slate (no pre-existing lockfile or fetched output).
Hide
Type `SRC="$PWD"; cd "$(mktemp -d)"; cp "$SRC/examples/command-system/.data.yaml" .; clear`
Enter
Sleep 500ms
Show

Type "cat .data.yaml"
Enter
Sleep 2.5s

Type "datum fetch"
Enter
Sleep 2s

Type "cut -d' ' -f1,3 data/system-info.txt"
Enter
Sleep 2s

Type "datum check"
Enter
Sleep 3s
