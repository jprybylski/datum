# Records the same file change checked under all three policies at once:
# `fail`, `update`, and `log`. Uses a scratch config (built by
# policy-reactions-setup.sh) with one local file tracked three times under
# three different policy settings, so a single edit shows how differently
# each one reacts to the exact same change.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/policy-reactions.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/policy-reactions.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 750
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

Hide
Type `cd "$(sh docs/assets/tapes/policy-reactions-setup.sh)"; clear`
Enter
Sleep 500ms
Show

Type "cat .data.yaml"
Enter
Sleep 3s

Type "datum fetch"
Enter
Sleep 2s

Type `echo "v2: changed content" > watched.txt`
Enter
Sleep 1.5s

Type "datum check"
Enter
Sleep 3s

Type "cat out/auto.txt out/strict.txt out/monitor.txt"
Enter
Sleep 3s
