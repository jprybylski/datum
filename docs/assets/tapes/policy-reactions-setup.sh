#!/bin/sh
# Sets up a scratch directory for the policy-reactions.tape recording: one
# local file (watched.txt) tracked three times under three different
# policies, so a single edit can show how `fail`, `update`, and `log` each
# react differently to the same upstream change.
#
# Prints the scratch directory's path on stdout so the caller can `cd` into
# it; every other line here is setup noise the recording shouldn't show.
set -e

dir=$(mktemp -d)
cd "$dir"

cat > .data.yaml <<'YAML'
version: 1
datasets:
  - id: strict
    desc: Fails the run when the file changes
    source: {type: file, path: watched.txt}
    target: out/strict.txt
    policy: fail
  - id: auto
    desc: Silently re-pins to the new content
    source: {type: file, path: watched.txt}
    target: out/auto.txt
    policy: update
  - id: monitor
    desc: Reports the change but doesn't fail or update
    source: {type: file, path: watched.txt}
    target: out/monitor.txt
    policy: log
YAML

echo "v1: initial content" > watched.txt

echo "$dir"
