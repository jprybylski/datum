# Records a live `datum fetch` + `datum check` run showing multi-source
# fallback: a dataset whose first source (an HTTP endpoint on a closed local
# port) always refuses the connection, so datum falls back to its second
# source (a local file) and fetches that instead.
#
# Uses a scratch config (built by multi-source-setup.sh) rather than
# examples/multi-source itself, whose `.data.yaml` uses illustrative but
# unreachable mirror URLs (example.com, mirror1-3.example.com) - fine to read
# as sample config, but not something a recording can run against
# deterministically.
#
# Run from the repo root, with a `datum` build on $PATH, e.g.:
#   go build -o bin/datum ./cmd/datum
#   PATH="$PWD/bin:$PATH" vhs docs/assets/tapes/multi-source.tape
#
# Regenerate this whenever the CLI's output format changes, so the GIF in the
# docs never drifts from what the tool actually prints.

Output docs/assets/img/multi-source.gif

Require datum

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 700
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 40ms

Hide
Type `cd "$(sh docs/assets/tapes/multi-source-setup.sh)"; clear`
Enter
Sleep 500ms
Show

Type "cat .data.yaml"
Enter
Sleep 3s

Type "datum fetch"
Enter
Sleep 2.5s

Type "cat data/app-config.json"
Enter
Sleep 2s

Type "datum check"
Enter
Sleep 3s
