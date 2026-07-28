#!/usr/bin/env bash
# Interactive sandbox for manually exercising datum's git handler against a real
# git-over-SSH server - a persistent counterpart to scripts/test-integration.sh, which brings
# the same server up only for the duration of `go test` and always tears it down afterward.
# This one stays up until you explicitly bring it down, so you can run `datum` against it by
# hand, `git clone` it, push more commits to it, etc. - and reset it back to a clean fixture
# state whenever you want. Requires Docker.
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE_FILE="test/integration/docker-compose.yml"
KEY_DIR="test/integration/testdata"
KEY_PATH="$KEY_DIR/id_ed25519"

usage() {
  cat <<EOF
Usage: $0 <up|down|reset|status>

  up      Start the sandbox git-over-SSH server, building it and generating an SSH
          keypair first if needed. Safe to run again if it's already up.
  down    Stop and remove the sandbox container (keeps the generated SSH key, so
          the next "up" reuses the same identity).
  reset   Tear the sandbox down and rebuild it from scratch, discarding any commits
          or changes you pushed to it while poking around - back to the pristine
          fixture repo (one commit on "main", tagged "v1.0.0").
  status  Show whether the sandbox is running and how to connect to it.
EOF
}

ensure_key() {
  mkdir -p "$KEY_DIR"
  if [ ! -f "$KEY_PATH" ]; then
    echo "Generating SSH keypair for the sandbox ($KEY_PATH)..."
    ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -C "datum-sandbox" -q
  fi
  cp "$KEY_PATH.pub" test/integration/gitserver/authorized_keys
}

is_running() {
  [ -n "$(docker compose -f "$COMPOSE_FILE" ps --status running --quiet 2>/dev/null)" ]
}

print_status() {
  cat <<EOF

Sandbox git server:  ssh://gituser@localhost:2222/srv/git/repo.git
  Fixture contents:  hello.txt on branch "main", tag "v1.0.0"
  SSH key:           $KEY_PATH

Clone it directly:
  GIT_SSH_COMMAND="ssh -i $KEY_PATH -o StrictHostKeyChecking=no" \\
    git clone ssh://gituser@localhost:2222/srv/git/repo.git

Or run datum against it (test/integration/sandbox.data.yaml has two ready-made datasets - one
pinned to "main", one to the "v1.0.0" tag). DATUM_GIT_INSECURE_HOST_KEY=1 is needed because this
throwaway sandbox host is intentionally not in your known_hosts. SSH_AUTH_SOCK is cleared so
datum uses this key instead of trying your real SSH agent's identities first (which the sandbox
server won't recognize):
  SSH_AUTH_SOCK= GIT_SSH_KEY=$PWD/$KEY_PATH DATUM_GIT_INSECURE_HOST_KEY=1 \\
    ./datum --config test/integration/sandbox.data.yaml \\
            --lock test/integration/sandbox-output/.data.lock.yaml fetch

Reset back to a clean fixture repo any time with: $0 reset
EOF
}

cmd="${1:-}"
case "$cmd" in
  up)
    ensure_key
    docker compose -f "$COMPOSE_FILE" up -d --build --wait
    echo "Sandbox is up."
    print_status
    ;;
  down)
    docker compose -f "$COMPOSE_FILE" down -v
    echo "Sandbox stopped. SSH key kept at $KEY_PATH - delete $KEY_DIR/ to also discard it."
    ;;
  reset)
    docker compose -f "$COMPOSE_FILE" down -v
    ensure_key
    docker compose -f "$COMPOSE_FILE" up -d --build --wait
    echo "Sandbox reset to a fresh fixture repo."
    print_status
    ;;
  status)
    if is_running; then
      echo "Sandbox is running."
      print_status
    else
      echo "Sandbox is not running. Start it with: $0 up"
    fi
    ;;
  *)
    usage
    exit 2
    ;;
esac
