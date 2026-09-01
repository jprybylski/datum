# Records a zero-argument interactive `datum init` run accepting the default project settings and
# declining an initial dataset. The minimal generated YAML is shown and accepted by `datum audit`.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -tags git -o /tmp/datum-tape-bin/datum ./cmd/datum
#   PATH="/tmp/datum-tape-bin:$PATH" vhs docs/assets/tapes/init.tape
#
# Regenerate this whenever init's output or generated YAML changes.

Output docs/assets/img/init.gif

Require datum

Set Shell "bash"
Set FontSize 21
Set Width 1400
Set Height 650
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

Hide
Type `cd "$(mktemp -d)"; clear`
Enter
Sleep 500ms
Show

Type "datum init"
Enter
Sleep 800ms

# Accept policy=fail, ignore=false, and add-initial-dataset=false.
Enter
Sleep 400ms
Enter
Sleep 400ms
Enter
Sleep 1.5s

Type "cat .data.yaml"
Enter
Sleep 3s

Type "datum audit"
Enter
Sleep 3s
