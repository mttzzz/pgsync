param(
    [ValidateSet("build", "build-all", "test", "pgtools-fetch", "pgtools-fetch-all", "pgtools-verify", "pgtools-sync", "pgtools-prepare-release", "package", "checksums", "release-local")]
    [string]$Task = "build",
    [string]$Version = "dev",
    [string]$Platform = "windows-amd64"
)

$ErrorActionPreference = "Stop"

function Invoke-BashScript([string]$Script, [string[]]$ArgsList = @()) {
    bash $Script @ArgsList
}

switch ($Task) {
    "build" {
        New-Item -ItemType Directory -Force -Path bin | Out-Null
        go build -ldflags "-X github.com/mttzzz/pgsync/internal/version.Version=$Version" -o bin/pgsync.exe ./cmd/pgsync
    }
    "build-all" {
        make build-all VERSION=$Version
    }
    "test" {
        go test -covermode=atomic -coverprofile=coverage.out ./internal/... ./pkg/...
    }
    "pgtools-fetch" {
        python scripts/fetch-pgtools-conda.py --platform $Platform
    }
    "pgtools-fetch-all" {
        python scripts/fetch-pgtools-conda.py --all
    }
    "pgtools-verify" {
        Invoke-BashScript "scripts/verify-pgtools.sh" @("embed/bin")
    }
    "pgtools-sync" {
        Invoke-BashScript "scripts/sync-pgtools-embed.sh"
    }
    "pgtools-prepare-release" {
        python scripts/fetch-pgtools-conda.py --all
        Invoke-BashScript "scripts/verify-pgtools.sh" @("embed/bin")
        Invoke-BashScript "scripts/sync-pgtools-embed.sh"
    }
    "package" {
        Invoke-BashScript "scripts/package-release.sh" @("--version", $Version)
    }
    "checksums" {
        Invoke-BashScript "scripts/checksums.sh" @("--dist", "dist")
    }
    "release-local" {
        go test -covermode=atomic -coverprofile=coverage.out ./internal/... ./pkg/...
        python scripts/fetch-pgtools-conda.py --all
        Invoke-BashScript "scripts/verify-pgtools.sh" @("embed/bin")
        Invoke-BashScript "scripts/sync-pgtools-embed.sh"
        Invoke-BashScript "scripts/package-release.sh" @("--version", $Version)
        Invoke-BashScript "scripts/checksums.sh" @("--dist", "dist")
    }
}
