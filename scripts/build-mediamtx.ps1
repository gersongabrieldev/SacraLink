<#
.SYNOPSIS
  Compila o MediaMTX (Windows x64) a partir do código-fonte oficial, na versão
  e no commit fixados.

.DESCRIPTION
  O binário publicado pelo projeto MediaMTX não é assinado. Para que o
  mediamtx.exe distribuído no instalador possa ser assinado junto com o
  SacraLink (exigência do Smart App Control e da SignPath Foundation, que só
  assina o que o CI compila do código-fonte), ele é compilado aqui com a
  mesma receita do projeto (scripts/binaries.mk do MediaMTX):
    go generate ./...  ->  CGO_ENABLED=0 go build  ->  go-winres (versão).
  A tag é conferida contra o commit fixado, para que uma tag movida não passe
  despercebida. Saída: <Destination>\mediamtx.exe, LICENSE e VERSION.

.EXAMPLE
  pwsh scripts/build-mediamtx.ps1
#>
param(
    [string]$Version = "v1.21.3",
    [string]$Commit = "9540cb6c1d9f1e9351fcc4ded3afc249ffff22bc",
    [string]$Destination = (Join-Path $PSScriptRoot "..\third_party\bin\mediamtx-source")
)

$ErrorActionPreference = "Stop"
# No GitHub Actions o erro vira anotação do job (visível sem login).
trap {
    if ($env:GITHUB_ACTIONS -eq "true") { Write-Host "::error title=MediaMTX::$($_.Exception.Message -replace "`r?`n", ' | ')" }
    break
}
# Roda um comando externo mostrando a saída; na falha, a mensagem leva as
# últimas linhas (a saída de erro de go/git não interrompe sozinha).
function Invoke-Native([string]$what, [scriptblock]$cmd) {
    $eap = $ErrorActionPreference; $ErrorActionPreference = "Continue"
    try {
        $out = @(& $cmd 2>&1 | ForEach-Object { "$_" })
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $eap }
    $out | Out-Host
    if ($code -ne 0) { throw "$what falhou (código $code): $(($out | Select-Object -Last 8) -join ' | ')" }
}

$src = Join-Path ([System.IO.Path]::GetTempPath()) ("sacralink-mediamtx-src-" + [guid]::NewGuid())
try {
    Write-Host "==> Código-fonte do MediaMTX $Version" -ForegroundColor Cyan
    Invoke-Native "git clone" { git -c advice.detachedHead=false clone --quiet --depth 1 --branch $Version https://github.com/bluenviron/mediamtx.git $src }
    $head = (git -C $src rev-parse HEAD).Trim()
    if ($head -ne $Commit) { throw "a tag $Version aponta para $head, esperado $Commit" }

    Push-Location $src
    $saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
    try {
        # Os geradores (go generate) rodam na máquina de build: sem GOOS/GOARCH.
        Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
        $env:CGO_ENABLED = "0"
        Write-Host "==> go generate (versão, hls.js)" -ForegroundColor Cyan
        Invoke-Native "go generate" { go generate ./... }
        $built = (Get-Content internal/core/VERSION -Raw).Trim()
        if ($built -ne $Version) { throw "versão gerada $built, esperado $Version" }

        Write-Host "==> go build (windows/amd64)" -ForegroundColor Cyan
        $env:GOOS = "windows"; $env:GOARCH = "amd64"
        Invoke-Native "go build" { go build -o mediamtx.exe . }
        Remove-Item Env:GOOS, Env:GOARCH

        Write-Host "==> Informações de versão do executável" -ForegroundColor Cyan
        $num = $Version.TrimStart("v") -replace '^(\d+\.\d+\.\d+).*$', '$1'
        Invoke-Native "go-winres" { go run github.com/tc-hib/go-winres@v0.3.3 patch --no-backup --in scripts/winres.json --product-version $num --file-version $num mediamtx.exe }
    } finally {
        foreach ($k in $saved.Keys) {
            if ($null -eq $saved[$k]) { Remove-Item "Env:$k" -ErrorAction SilentlyContinue } else { Set-Item "Env:$k" $saved[$k] }
        }
        Pop-Location
    }

    if (Test-Path $Destination) { Remove-Item -Recurse -Force $Destination }
    New-Item -ItemType Directory -Force $Destination | Out-Null
    Copy-Item (Join-Path $src "mediamtx.exe") $Destination
    Copy-Item (Join-Path $src "LICENSE") $Destination
    Set-Content -Path (Join-Path $Destination "VERSION") -Value "$Version ($Commit, compilado do código-fonte)" -Encoding utf8
    $hash = (Get-FileHash (Join-Path $Destination "mediamtx.exe") -Algorithm SHA256).Hash.ToLower()
    Write-Host "MediaMTX $Version compilado: $Destination (SHA-256 $hash)" -ForegroundColor Green
} finally {
    Remove-Item -Recurse -Force $src -ErrorAction SilentlyContinue
}
