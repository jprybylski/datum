#!/bin/bash
# Runs as gituser at image build time to populate the bare fixture repo served over SSH.
set -euo pipefail

export GIT_AUTHOR_NAME="datum-integration-test"
export GIT_AUTHOR_EMAIL="datum-integration-test@example.com"
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"

WORK=$(mktemp -d)
git init -q -b main "$WORK"
cd "$WORK"
echo "hello from the integration test git server" > hello.txt
git add hello.txt
git commit -q -m "initial commit"
git tag v1.0.0

git init -q --bare /srv/git/repo.git
git push -q /srv/git/repo.git main
git push -q /srv/git/repo.git refs/tags/v1.0.0

cd /
rm -rf "$WORK"
