<#
.SYNOPSIS
  Baixa o binário oficial do MediaMTX (versão fixada) e verifica o SHA-256.

.DESCRIPTION
  Usado no desenvolvimento e no CI do instalador. O binário é salvo em
  third_party/bin/mediamtx/ (fora do controle de versão). A soma SHA-256 é
  conferida contra o arquivo checksums.sha256 publicado na release oficial.

.EXAMPLE
  pwsh scripts/fetch-mediamtx.ps1
#>
param(
    [string]$Version = "v1.21.3",
    [string]$Destination = (Join-Path $PSScriptRoot "..\third_party\bin\mediamtx")
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$asset = "mediamtx_${Version}_windows_amd64.zip"
$base = "https://github.com/bluenviron/mediamtx/releases/download/$Version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("sacralink-mediamtx-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force $tmp | Out-Null

try {
    Write-Host "Baixando $asset ..."
    Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset)
    Invoke-WebRequest "$base/checksums.sha256" -OutFile (Join-Path $tmp "checksums.sha256")

    $line = Get-Content (Join-Path $tmp "checksums.sha256") | Where-Object { $_ -match [regex]::Escape($asset) }
    if (-not $line) { throw "Checksum de $asset não encontrado em checksums.sha256" }
    $expected = ($line -split '\s+')[0].ToLower()
    $actual = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { throw "SHA-256 divergente: esperado $expected, obtido $actual" }
    Write-Host "SHA-256 conferido: $actual"

    New-Item -ItemType Directory -Force $Destination | Out-Null
    Expand-Archive (Join-Path $tmp $asset) -DestinationPath $Destination -Force
    Set-Content -Path (Join-Path $Destination "VERSION") -Value $Version -Encoding ascii
    Write-Host "MediaMTX $Version instalado em $(Resolve-Path $Destination)"
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
