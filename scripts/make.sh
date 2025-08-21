set -euo pipefail
TAGS="${1:-}"
go mod tidy
go vet ./...
go build -tags "${TAGS}" -o ./bin/pinup ./cmd/pinup
echo "Built bin/pinup (tags: ${TAGS})"
