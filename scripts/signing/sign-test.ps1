<#
.SYNOPSIS
  Assina os executáveis de uma pasta com um certificado DESCARTÁVEL, só para
  testar o fluxo de assinatura no CI.

.DESCRIPTION
  Cria (na máquina efêmera do GitHub Actions) um certificado autoassinado e
  assina todos os .exe/.com da pasta, simulando a SignPath. O Windows não
  confia nesse certificado: ele serve apenas para validar a mecânica (as duas
  compilações do instalador, a troca pelas cópias assinadas e a conferência
  final). Nunca é usado em releases.
#>
param([Parameter(Mandatory)][string]$Path)
$ErrorActionPreference = "Stop"

$subject = "CN=SacraLink - assinatura de teste (descartavel)"
$cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object Subject -eq $subject | Select-Object -First 1
if (-not $cert) {
    $cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject $subject -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddDays(1)
}
$files = Get-ChildItem -LiteralPath $Path -Recurse -File | Where-Object { $_.Extension -in ".exe", ".com" }
if (-not $files) { throw "nenhum executável para assinar em $Path" }
foreach ($f in $files) {
    $r = Set-AuthenticodeSignature -LiteralPath $f.FullName -Certificate $cert -HashAlgorithm SHA256
    if (-not $r.SignerCertificate) { throw "falha ao assinar $($f.Name): $($r.StatusMessage)" }
    Write-Host "assinado (teste): $($f.Name)"
}
