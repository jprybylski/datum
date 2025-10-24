set -euo pipefail
TAGS="${1:-}"
go mod tidy
go vet ./...
go build -tags "${TAGS}" -o ./bin/datum ./cmd/datum
echo "Built bin/datum (tags: ${TAGS})"
