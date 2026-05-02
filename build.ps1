param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -ldflags "-X github.com/mttzzz/pgsync/internal/version.Version=$Version" -o bin/pgsync.exe ./cmd/pgsync
