<#
.SYNOPSIS
  Baixa o FFmpeg (build "essentials" de gyan.dev) para TESTES de desenvolvimento.

.DESCRIPTION
  O FFmpeg NÃO faz parte do produto nem do instalador. Ele é usado apenas para
  gerar um padrão de teste H.264 + Opus e publicá-lo via WHIP/RTSP, simulando um
  celular nos testes ponta a ponta (scripts/dev/publish-test-pattern.ps1).
  O binário fica em third_party/bin/ffmpeg/ (fora do controle de versão).
#>
param(
    [string]$Destination = (Join-Path $PSScriptRoot "..\..\third_party\bin\ffmpeg")
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$url = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("sacralink-ffmpeg-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force $tmp | Out-Null
try {
    $zip = Join-Path $tmp "ffmpeg.zip"
    Invoke-WebRequest $url -OutFile $zip
    $expected = (Invoke-WebRequest "$url.sha256" -UseBasicParsing).Content.Trim().Split(" ")[0].ToLower()
    $actual = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { throw "SHA-256 divergente: esperado $expected, obtido $actual" }
    Write-Host "SHA-256 conferido: $actual"
    Expand-Archive $zip -DestinationPath $tmp
    $bin = Get-ChildItem $tmp -Recurse -Filter ffmpeg.exe | Select-Object -First 1
    New-Item -ItemType Directory -Force $Destination | Out-Null
    Copy-Item (Join-Path $bin.DirectoryName "*") $Destination -Force
    & (Join-Path $Destination "ffmpeg.exe") -hide_banner -version | Select-Object -First 1
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
