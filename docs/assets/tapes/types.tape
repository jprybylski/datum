# Records `datum types` listing the source types compiled into a Git-enabled release build, then
# showing the schema-derived fields for selected types and the machine-readable JSON form.
#
# Run from the repo root, with a Git-enabled `datum` build on $PATH, e.g.:
#   go build -tags git -o /tmp/datum-vhs/datum ./cmd/datum
#   PATH="/tmp/datum-vhs:$PATH" vhs docs/assets/tapes/types.tape
#
# Regenerate this whenever the schema or types output changes, so the GIF remains an accurate
# recording of the release binary.

Output docs/assets/img/types.gif

Require datum

Set Shell "bash"
Set FontSize 20
Set Width 1400
Set Height 800
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

Type "datum types"
Enter
Sleep 3s

Type "datum types http"
Enter
Sleep 4s

Type "datum --json types http"
Enter
Sleep 4s
