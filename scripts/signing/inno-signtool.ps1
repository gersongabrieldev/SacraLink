<#
.SYNOPSIS
  "SignTool" do Inno Setup para assinatura remota (SignPath) em duas passadas.

.DESCRIPTION
  O Inno Setup só assina o programa temporário que o instalador extrai ao rodar
  (e o desinstalador) DURANTE a compilação, chamando o SignTool para cada
  arquivo, e confere se ele voltou assinado. A SignPath, porém, assina
  artefatos do GitHub Actions, fora do ISCC. Por isso o instalador é
  compilado duas vezes:

    collect  guarda a cópia SEM assinatura de cada parte interna em
             <Dir>\<sha256>.exe (é ela que vai para a SignPath) e devolve ao
             ISCC o arquivo com assinatura descartável, só para a compilação
             seguir (esse instalador é descartado);
    replay   na segunda compilação, idêntica, troca cada parte interna pela
             cópia assinada de mesmo SHA-256 (registrada em used.txt); uma
             parte sem cópia assinada indica compilação diferente da coleta
             (unknown.txt) e faz o build falhar.

  O instalador final (SacraLink-Setup-x64.exe) também passa por aqui no fim
  de cada compilação: no replay, a versão SEM assinatura é guardada em
  <Dir>\final\ para a rodada de assinatura seguinte; o ISCC recebe a cópia
  com assinatura descartável.
#>
param(
    [Parameter(Mandatory)][ValidateSet("collect", "replay")][string]$Mode,
    [Parameter(Mandatory)][string]$Dir,
    [Parameter(Mandatory)][string]$Target
)
$ErrorActionPreference = "Stop"
$testSign = Join-Path $PSScriptRoot "sign-test.ps1"

$leaf = Split-Path -Leaf $Target
$hash = (Get-FileHash -LiteralPath $Target -Algorithm SHA256).Hash.ToLower()
# Registro lido pelo build-installer.ps1 (o ISCC não repassa esta saída).
function Write-Log([string]$msg) {
    Add-Content -LiteralPath (Join-Path $Dir "signtool-log.txt") -Value "$Mode $leaf $($hash.Substring(0, 12)) $msg"
}
New-Item -ItemType Directory -Force $Dir | Out-Null

if ($leaf -eq "SacraLink-Setup-x64.exe") {
    if ($Mode -eq "replay") {
        $final = Join-Path $Dir "final"
        New-Item -ItemType Directory -Force $final | Out-Null
        Copy-Item -LiteralPath $Target -Destination (Join-Path $final $leaf) -Force
        Write-Log "instalador final guardado sem assinatura"
    } else {
        Write-Log "instalador final da coleta (descartado)"
    }
    & $testSign -Path $Target
    exit 0
}

$copy = Join-Path $Dir "$hash.exe"
if ($Mode -eq "collect") {
    Copy-Item -LiteralPath $Target -Destination $copy -Force
    & $testSign -Path $Target
    Write-Log "separado ($((Get-Item -LiteralPath $copy).Length) bytes)"
    exit 0
}

if (Test-Path -LiteralPath $copy) {
    Copy-Item -LiteralPath $copy -Destination $Target -Force
    Add-Content -LiteralPath (Join-Path $Dir "used.txt") -Value $hash
    Write-Log "cópia assinada aplicada"
} else {
    Add-Content -LiteralPath (Join-Path $Dir "unknown.txt") -Value "$hash $leaf"
    Write-Log "SEM cópia assinada"
    & $testSign -Path $Target # deixa o ISCC terminar; o build falha na conferência
}
exit 0
