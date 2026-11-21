<#
.SYNOPSIS
  "SignTool" do Inno Setup para assinatura remota (SignPath) em duas passadas.

.DESCRIPTION
  O Inno Setup só assina o programa temporário que o instalador extrai ao rodar
  (e o desinstalador) DURANTE a compilação, chamando o SignTool para cada
  arquivo. A SignPath, porém, assina artefatos do GitHub Actions, fora do
  ISCC. Por isso o instalador é compilado duas vezes:

    collect  copia cada arquivo que o ISCC pede para assinar para
             <Dir>\<sha256>.exe (o ISCC segue com o original); a pasta vai
             para a SignPath;
    replay   na segunda compilação, idêntica, troca cada arquivo pela cópia
             assinada de mesmo SHA-256 e registra em <Dir>\used.txt. Um
             arquivo sem cópia assinada indica compilação diferente da coleta
             e é registrado em <Dir>\unknown.txt.

  O SacraLink-Setup-x64.exe final não passa por aqui: é assinado numa rodada
  própria, depois do ISCC (scripts/build-installer.ps1).
#>
param(
    [Parameter(Mandatory)][ValidateSet("collect", "replay")][string]$Mode,
    [Parameter(Mandatory)][string]$Dir,
    [Parameter(Mandatory)][string]$Target
)
$ErrorActionPreference = "Stop"

$leaf = Split-Path -Leaf $Target
if ($leaf -eq "SacraLink-Setup-x64.exe") {
    Write-Host "instalador final: assinado na rodada seguinte"
    exit 0
}
$hash = (Get-FileHash -LiteralPath $Target -Algorithm SHA256).Hash.ToLower()
$copy = Join-Path $Dir "$hash.exe"
# Registro lido pelo build-installer.ps1 (o ISCC não repassa esta saída).
function Write-Log([string]$msg) {
    Add-Content -LiteralPath (Join-Path $Dir "signtool-log.txt") -Value "$Mode $leaf $($hash.Substring(0, 12)) $msg"
}

if ($Mode -eq "collect") {
    New-Item -ItemType Directory -Force $Dir | Out-Null
    Copy-Item -LiteralPath $Target -Destination $copy -Force
    Write-Log "separado ($((Get-Item -LiteralPath $Target).Length) bytes)"
    exit 0
}

if (Test-Path -LiteralPath $copy) {
    Copy-Item -LiteralPath $copy -Destination $Target -Force
    Add-Content -LiteralPath (Join-Path $Dir "used.txt") -Value $hash
    Write-Log "cópia assinada aplicada"
} else {
    Add-Content -LiteralPath (Join-Path $Dir "unknown.txt") -Value "$hash $leaf"
    Write-Log "SEM cópia assinada"
}
exit 0
