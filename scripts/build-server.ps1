<#
.SYNOPSIS
  Compila o sacralink-server.exe (e o sl-testpublisher.exe) para Windows x64.

.DESCRIPTION
  Build reproduzível: CGO desativado, -trimpath, versão/commit/data injetados.
  O servidor é compilado no subsistema GUI do Windows (sem janela de console).

.EXAMPLE
  pwsh scripts/build-server.ps1 -Version 0.1.0
#>
param(
    [string]$Version = "0.0.0-dev",
    [string]$Output = (Join-Path $PSScriptRoot "..\dist-release\server")
)
$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$commit = (git -C $root rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$pkg = "github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo"
$ldflags = "-s -w -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date"

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
New-Item -ItemType Directory -Force $Output | Out-Null
Push-Location (Join-Path $root "apps\server")
try {
    go build -trimpath -ldflags "$ldflags -H windowsgui" -o (Join-Path $Output "sacralink-server.exe") ./cmd/sacralink-server
    if ($LASTEXITCODE -ne 0) { throw "falha no build do servidor" }
    go build -trimpath -ldflags $ldflags -o (Join-Path $Output "sl-testpublisher.exe") ./cmd/sl-testpublisher
    if ($LASTEXITCODE -ne 0) { throw "falha no build do sl-testpublisher" }
}
finally { Pop-Location }
Get-ChildItem $Output | Select-Object Name, Length
