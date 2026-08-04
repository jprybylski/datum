#!/bin/sh
# Sets up a scratch directory for the audit.tape recording: four datasets
# whose config/lockfile end up in each of the four states datum audit
# reports (ok, pending, deleted, orphaned), plus a second config
# (.data.yaml.without-orphan) the recording swaps in to simulate a user
# editing to_be_removed out of .data.yaml by hand.
#
# Prints the scratch directory's path on stdout so the caller can `cd` into
# it; every other line here is setup noise the recording shouldn't show.
set -e

dir=$(mktemp -d)
cd "$dir"

echo "ok content" > src_ok.txt
echo "pending content" > src_pending.txt
echo "deleted content" > src_deleted.txt
echo "orphan content" > src_orphan.txt

cat > .data.yaml <<'YAML'
version: 1
defaults:
  policy: update
datasets:
  - id: tracked_ok
    source: {type: file, path: src_ok.txt}
    target: out/ok.txt
  - id: tracked_pending
    source: {type: file, path: src_pending.txt}
    target: out/pending.txt
  - id: tracked_deleted
    source: {type: file, path: src_deleted.txt}
    target: out/deleted.txt
  - id: to_be_removed
    source: {type: file, path: src_orphan.txt}
    target: out/orphan.txt
YAML

cat > .data.yaml.without-orphan <<'YAML'
version: 1
defaults:
  policy: update
datasets:
  - id: tracked_ok
    source: {type: file, path: src_ok.txt}
    target: out/ok.txt
  - id: tracked_pending
    source: {type: file, path: src_pending.txt}
    target: out/pending.txt
  - id: tracked_deleted
    source: {type: file, path: src_deleted.txt}
    target: out/deleted.txt
YAML

echo "$dir"
