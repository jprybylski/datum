#!/bin/sh
# Sets up a scratch directory for the multi-source.tape recording: one
# dataset with two sources, an HTTP endpoint that always refuses the
# connection and a local file fallback, so a single `datum fetch` can show
# the first source failing over to the second deterministically (no real
# network flakiness or unreachable-mirror placeholders like
# examples/multi-source's `.data.yaml` uses for illustration).
#
# Prints the scratch directory's path on stdout so the caller can `cd` into
# it; every other line here is setup noise the recording shouldn't show.
set -e

dir=$(mktemp -d)
cd "$dir"

mkdir -p backups

cat > backups/app-config.json <<'JSON'
{
  "app_name": "Example Application",
  "environment": "production"
}
JSON

cat > .data.yaml <<'YAML'
version: 1
datasets:
  - id: app_config
    desc: Remote config with a local fallback if the server is unreachable
    sources:
      - type: http
        url: http://127.0.0.1:9/app-config.json
      - type: file
        path: backups/app-config.json
    target: data/app-config.json
    policy: update
YAML

echo "$dir"
