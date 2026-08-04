# Records a live `datum audit` run showing all four states it reports:
# ok (tracked_ok), pending (tracked_pending, deliberately never fetched),
# deleted (tracked_deleted, via `datum delete`), and orphaned (to_be_removed,
# simulated by swapping in a .data.yaml that no longer mentions it). Uses a
# scratch config built by audit-setup.sh. Finishes with `datum --json audit`
# to show the machine-readable form too.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/audit.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/audit.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 850
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

Hide
Type `cd "$(sh docs/assets/tapes/audit-setup.sh)"; clear`
Enter
Sleep 500ms
Show

Type "datum fetch tracked_ok tracked_deleted to_be_removed"
Enter
Sleep 2.5s

Type "datum audit"
Enter
Sleep 3s

Type "datum --yes delete tracked_deleted"
Enter
Sleep 2s

Type "cp .data.yaml.without-orphan .data.yaml"
Enter
Sleep 1s

Type "datum audit"
Enter
Sleep 4s

Type "datum --json audit"
Enter
Sleep 4s
