# Builds Tilqos on Windows.
#
#   .\build.ps1          sync models, check icons, test, vet and build amd64 + arm64
#   .\build.ps1 -Fast    the same without running the tests
#
# If script execution is disabled:  powershell -ExecutionPolicy Bypass -File .\build.ps1
#
# Output in dist\:
#   Tilqos-amd64.exe, Tilqos-arm64.exe          the bare executables
#   Tilqos-<version>-Windows-x64.zip            Tilqos.exe + LICENSE + THIRD_PARTY_NOTICES.md
#                                               + NOTICE + LICENSES\ (x64)
#   Tilqos-<version>-Windows-arm64.zip          the same for ARM64
#
# <version> defaults to the Version constant in internal\winapp\app_windows.go; override it
# with $env:VERSION for a one-off build.
# Needs only Go 1.25+ (https://go.dev/dl/); nothing is downloaded, no C compiler needed.
param([switch]$Fast)

$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

function Step([string]$Message) { Write-Host "==> $Message" -ForegroundColor Cyan }
function Check([string]$What) { if ($LASTEXITCODE -ne 0) { throw "$What failed (exit code $LASTEXITCODE)" } }

$version = $env:VERSION
if (-not $version) {
    $line = Select-String -Path 'internal\winapp\app_windows.go' -Pattern '^var Version = "(.*)"$' | Select-Object -First 1
    if ($line) { $version = $line.Matches[0].Groups[1].Value }
}
if (-not $version) { throw 'cannot determine the version' }
$ldflags = "-H windowsgui -s -w -X tilqos/internal/winapp.Version=$version"

# The models are embedded from copies of ..\Resources\Models, the single source of truth.
Step 'Syncing models from ..\Resources\Models'
go run ./tools/syncmodels; Check 'syncmodels'
go run ./tools/syncmodels -check; Check 'syncmodels -check'

# The license texts are embedded too (Licenses in the tray menu; a lone .exe keeps its notices).
Step 'Syncing LICENSE and THIRD_PARTY_NOTICES.md from ..'
go run ./tools/synclicenses; Check 'synclicenses'
go run ./tools/synclicenses -check; Check 'synclicenses -check'

# Icons are committed; regenerate only if Resources\icon-1024.png changed.
go run ./tools/mkico -check 2>$null
if ($LASTEXITCODE -ne 0) {
    Step 'Regenerating icons from ..\Resources\icon-1024.png'
    go run ./tools/mkico; Check 'mkico'
}

if (-not $Fast) {
    Step 'Testing (includes parity with the Swift implementation)'
    go test ./...; Check 'go test'
}

New-Item -ItemType Directory -Force -Path dist | Out-Null
$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
try {
    foreach ($arch in @('amd64', 'arm64')) {
        $env:GOOS = 'windows'
        $env:GOARCH = $arch
        $env:CGO_ENABLED = '0'
        Step "Vetting windows/$arch"
        go vet ./...; Check "go vet ($arch)"
        Step "Building windows/$arch ($version)"
        go build -trimpath -ldflags $ldflags -o "dist/Tilqos-$arch.exe" ./cmd/tilqos; Check "go build ($arch)"
    }
} finally {
    foreach ($name in $saved.Keys) {
        if ($null -eq $saved[$name]) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
        else { Set-Item "Env:$name" $saved[$name] }
    }
}

foreach ($arch in @('amd64', 'arm64')) {
    $label = if ($arch -eq 'amd64') { 'x64' } else { $arch }
    $archive = "dist\Tilqos-$version-Windows-$label.zip"
    $stage = Join-Path ([System.IO.Path]::GetTempPath()) ("tilqos-" + [System.Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force -Path $stage | Out-Null
    try {
        Copy-Item "dist\Tilqos-$arch.exe" (Join-Path $stage 'Tilqos.exe')
        foreach ($doc in @('LICENSE', 'THIRD_PARTY_NOTICES.md')) {
            Copy-Item "..\$doc" $stage
        }
        Copy-Item '..\Resources\Models\NOTICE' (Join-Path $stage 'NOTICE')
        if (Test-Path '..\LICENSES') { Copy-Item '..\LICENSES' (Join-Path $stage 'LICENSES') -Recurse }
        Step "Packaging $archive"
        if (Test-Path $archive) { Remove-Item $archive }
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive
    } finally {
        Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
    }
}

Step 'Done'
Get-ChildItem dist\Tilqos-* | Format-Table Name, Length
