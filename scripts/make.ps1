param([string]$Tags="")
$ErrorActionPreference="Stop"
go mod tidy
go vet ./...
go build -tags "$Tags" -o .\bin\datum.exe .\cmd\datum
Write-Host "Built bin\datum.exe (tags: $Tags)"
