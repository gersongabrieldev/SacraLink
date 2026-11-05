<#
.SYNOPSIS
  Build completo do SacraLink para Windows x64 (sem a CLI do Wails).

.DESCRIPTION
  1. Gera os ícones a partir da identidade visual (internal/assets).
  2. Compila o painel (React/Vite) — Node.js necessário só no build.
  3. Gera os recursos Windows (.syso: ícone, manifesto DPI, versão).
  4. Compila SacraLink.exe (Wails v2) e sacralink-server.exe (Go, sem CGO).
  5. Copia o MediaMTX (versão fixada) com o arquivo .sha256 de integridade.
  Saída: dist-release\SacraLink\ (pronta para o instalador Inno Setup).

.EXAMPLE
  pwsh scripts/build-windows.ps1 -Version 0.1.0
#>
param(
    [string]$Version = "0.0.0-dev",
    [switch]$SkipFrontend
)
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$out = Join-Path $root "dist-release\SacraLink"
$commit = (git -C $root rev-parse --short HEAD 2>$null); if (-not $commit) { $commit = "unknown" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$env:CGO_ENABLED = "0"; $env:GOOS = "windows"; $env:GOARCH = "amd64"
if (-not $env:GOTMPDIR) { $env:GOTMPDIR = Join-Path $env:USERPROFILE ".gotmp"; New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null }

function Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Check($what) { if ($LASTEXITCODE -ne 0) { throw "falhou: $what" } }

if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Force $out | Out-Null

Step "Ícones"
Push-Location (Join-Path $root "apps\server")
go run ./cmd/sl-genicons -out (Join-Path $root "assets\icons"); Check "ícones"
Pop-Location

if (-not $SkipFrontend) {
    Step "Painel (frontend)"
    Push-Location (Join-Path $root "apps\desktop\frontend")
    npm ci --no-audit --no-fund; Check "npm ci"
    npm run build; Check "build do frontend"
    Pop-Location
}

Step "Recursos Windows"
Push-Location (Join-Path $root "apps\desktop")
$ico = Join-Path $root "assets\icons\sacralink.ico"
go run ./tools/genres -icon $ico -kind painel -version $Version -out rsrc_windows_amd64.syso; Check "recurso do painel"
go run ./tools/genres -icon $ico -kind servidor -version $Version -out (Join-Path $root "apps\server\cmd\sacralink-server\rsrc_windows_amd64.syso"); Check "recurso do servidor"

Step "SacraLink.exe (painel Wails)"
go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui -X main.Version=$Version" -o (Join-Path $out "SacraLink.exe") .; Check "painel"
Pop-Location

Step "sacralink-server.exe"
Push-Location (Join-Path $root "apps\server")
$pkg = "github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo"
go build -trimpath -ldflags "-s -w -H windowsgui -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date" -o (Join-Path $out "sacralink-server.exe") ./cmd/sacralink-server; Check "servidor"
Pop-Location

Step "Painel para navegador (servido pelo servidor)"
Copy-Item -Recurse (Join-Path $root "apps\desktop\frontend\dist") (Join-Path $out "dashboard")

Step "MediaMTX"
$mtx = Join-Path $root "third_party\bin\mediamtx"
if (-not (Test-Path (Join-Path $mtx "mediamtx.exe"))) { & (Join-Path $root "scripts\fetch-mediamtx.ps1") }
Copy-Item (Join-Path $mtx "mediamtx.exe") $out
Copy-Item (Join-Path $mtx "LICENSE") (Join-Path $out "mediamtx-LICENSE.txt")
$hash = (Get-FileHash (Join-Path $out "mediamtx.exe") -Algorithm SHA256).Hash.ToLower()
Set-Content -Path (Join-Path $out "mediamtx.exe.sha256") -Value "$hash  mediamtx.exe" -Encoding ascii

Step "Documentos"
Copy-Item (Join-Path $root "LICENSE") (Join-Path $out "LICENSE.txt")
Copy-Item (Join-Path $root "assets\icons\sacralink.ico") $out

Get-ChildItem $out | Select-Object Name, Length | Format-Table -AutoSize
Write-Host "Build pronto em $out" -ForegroundColor Green
