param([string]$Tags="")
$ErrorActionPreference="Stop"
go mod tidy
go vet ./...
go build -tags "$Tags" -o .\bin\pinup.exe .\cmd\pinup
Write-Host "Built bin\pinup.exe (tags: $Tags)"
