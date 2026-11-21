<#
.SYNOPSIS
  Assina com um certificado DESCARTÁVEL (autoassinado, criado na hora).

.DESCRIPTION
  Usos:
  - no CI, simula a SignPath para testar o fluxo de assinatura completo;
  - durante a compilação do instalador (scripts/signing/inno-signtool.ps1),
    satisfaz a conferência do ISCC, que exige que o SignTool devolva o arquivo
    assinado. Esses arquivos são sempre substituídos pelas versões assinadas
    pela SignPath (ou descartados) antes da publicação; a conferência final
    da release exige assinatura confiável no Windows.
  O Windows não confia nesse certificado. -Path aceita uma pasta (assina os
  .exe/.com) ou um arquivo (qualquer extensão).
#>
param([Parameter(Mandatory)][string]$Path)
$ErrorActionPreference = "Stop"

$subject = "CN=SacraLink - assinatura de teste (descartavel)"
$cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object Subject -eq $subject | Select-Object -First 1
if (-not $cert) {
    $cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject $subject -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddDays(1)
}

function Set-TestSignature([string]$file) {
    # Assina uma cópia .exe (a extensão não importa para a assinatura de um PE,
    # mas o Windows escolhe o formato pela extensão) e devolve no lugar.
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("sl-sign-" + [guid]::NewGuid() + ".exe")
    Copy-Item -LiteralPath $file -Destination $tmp
    try {
        $r = Set-AuthenticodeSignature -LiteralPath $tmp -Certificate $cert -HashAlgorithm SHA256
        if (-not $r.SignerCertificate) { throw "falha ao assinar $(Split-Path -Leaf $file): $($r.StatusMessage)" }
        Copy-Item -LiteralPath $tmp -Destination $file -Force
    } finally {
        Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    }
}

if (Test-Path -LiteralPath $Path -PathType Leaf) {
    Set-TestSignature $Path
    exit 0
}
$files = Get-ChildItem -LiteralPath $Path -Recurse -File | Where-Object { $_.Extension -in ".exe", ".com" }
if (-not $files) { throw "nenhum executável para assinar em $Path" }
foreach ($f in $files) {
    Set-TestSignature $f.FullName
    Write-Host "assinado (teste): $($f.Name)"
}
