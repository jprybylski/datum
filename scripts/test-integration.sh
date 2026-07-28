#!/usr/bin/env bash
# Runs the Docker-backed integration test suite (test/integration/, gated behind the
# "integration" build tag). Requires Docker; not part of the regular `go test ./...` run.
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE_FILE="test/integration/docker-compose.yml"
KEY_DIR="test/integration/testdata"
KEY_PATH="$KEY_DIR/id_ed25519"

mkdir -p "$KEY_DIR"
if [ ! -f "$KEY_PATH" ]; then
  echo "Generating ephemeral SSH keypair for this test run ($KEY_PATH)..."
  ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -C "datum-integration-test-ephemeral" -q
fi
cp "$KEY_PATH.pub" test/integration/gitserver/authorized_keys

cleanup() {
  echo "Tearing down integration test containers..."
  docker compose -f "$COMPOSE_FILE" down -v
}
trap cleanup EXIT

echo "Building and starting the integration test git server..."
docker compose -f "$COMPOSE_FILE" up -d --build --wait

export DATUM_TEST_GIT_SSH_KEY="$PWD/$KEY_PATH"
echo "Running integration tests..."
go test -tags integration ./test/integration/... -v

echo "Integration tests passed."
