<#
.SYNOPSIS
  Monta o instalador SacraLink-Setup-x64.exe (Inno Setup) e conduz as rodadas
  de assinatura.

.DESCRIPTION
  Roda depois de scripts/build-windows.ps1. Sem assinatura, basta a fase
  "compile". Com assinatura, o release.yml alterna estas fases com os pedidos
  à SignPath (que assina pastas enviadas como artefatos do GitHub Actions):

    stage-binaries  copia os executáveis de dist-release\SacraLink para -Dir
                    (rodada 1: SacraLink.exe, servidor, .com e MediaMTX)
    apply-binaries  devolve os executáveis assinados de -Dir e refaz o
                    mediamtx.exe.sha256 (a assinatura muda o hash)
    compile         compila o instalador. -Mode plain: sem assinatura;
                    collect: separa em -Dir o programa temporário e o
                    desinstalador que o Inno Setup assina (rodada 2);
                    replay: compila de novo usando as cópias assinadas de -Dir
    stage-setup     copia o instalador final para -Dir (rodada 3)
    apply-setup     devolve o instalador assinado de -Dir
    verify          confere que os executáveis e o instalador estão assinados
                    (-RequireTrusted: com certificado confiável no Windows)
    checksums       gera o SHA256SUMS.txt
    test            fluxo completo com um certificado descartável (CI), para
                    validar a mecânica antes de existir a assinatura real

.EXAMPLE
  pwsh scripts/build-installer.ps1 -Phase compile -Version 0.2.0
  pwsh scripts/build-installer.ps1 -Phase test -Version 0.0.0-ci.1
#>
param(
    [Parameter(Mandatory)]
    [ValidateSet("stage-binaries", "apply-binaries", "compile", "stage-setup", "apply-setup", "verify", "checksums", "test")]
    [string]$Phase,
    [string]$Version = "0.0.0-dev",
    [ValidateSet("plain", "collect", "replay")][string]$Mode = "plain",
    [string]$Dir,
    [switch]$RequireTrusted
)
$ErrorActionPreference = "Stop"
# No GitHub Actions, erros e o registro do SignTool viram anotações do job
# (visíveis sem login, ao contrário dos logs).
$gha = $env:GITHUB_ACTIONS -eq "true"
function Write-Annotation([string]$kind, [string]$msg) {
    if ($gha) { Write-Host "::$kind title=Instalador::$($msg -replace "`r?`n", ' | ')" } else { Write-Host $msg }
}
trap { Write-Annotation "error" "fase ${Phase}: $($_.Exception.Message)"; break }
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$dist = Join-Path $root "dist-release\SacraLink"
$output = Join-Path $root "installer\windows\Output"
$setupName = "SacraLink-Setup-x64.exe"
$binaries = "SacraLink.exe", "sacralink-server.exe", "sacralink-server.com", "mediamtx.exe"

function Full([string]$p) {
    if (-not $p) { throw "informe -Dir" }
    $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($p)
}
function New-Empty([string]$p) {
    if (Test-Path $p) { Remove-Item -Recurse -Force $p }
    New-Item -ItemType Directory -Force $p | Out-Null
}
# Procura o arquivo na pasta devolvida pela SignPath (que pode ter subpastas).
function Find-In([string]$d, [string]$name) {
    $f = Get-ChildItem -LiteralPath $d -Recurse -File -Filter $name | Select-Object -First 1
    if (-not $f) { throw "$name não encontrado em $d" }
    $f.FullName
}
function Find-Iscc {
    $c = Get-Command iscc -ErrorAction SilentlyContinue
    if ($c) { return $c.Source }
    $p = "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe"
    if (Test-Path $p) { return $p }
    throw "Inno Setup (ISCC) não encontrado"
}
function Get-Numeric([string]$v) {
    $n = @(($v -split "-")[0].Split("."))
    while ($n.Count -lt 4) { $n += "0" }
    ($n[0..3]) -join "."
}
function Invoke-Self([hashtable]$a) { & $PSCommandPath @a } # erros interrompem (ErrorActionPreference)

switch ($Phase) {
    "stage-binaries" {
        $d = Full $Dir; New-Empty $d
        foreach ($b in $binaries) { Copy-Item (Join-Path $dist $b) $d }
        Write-Host "Para assinar (rodada 1): $($binaries -join ', ')"
    }
    "apply-binaries" {
        $d = Full $Dir
        foreach ($b in $binaries) { Copy-Item (Find-In $d $b) (Join-Path $dist $b) -Force }
        $hash = (Get-FileHash (Join-Path $dist "mediamtx.exe") -Algorithm SHA256).Hash.ToLower()
        Set-Content -Path (Join-Path $dist "mediamtx.exe.sha256") -Value "$hash  mediamtx.exe" -Encoding ascii
        Write-Host "Executáveis assinados aplicados; mediamtx.exe.sha256 atualizado"
    }
    "compile" {
        $isccArgs = @("/DAppVersion=$Version", "/DAppVersionNumeric=$(Get-Numeric $Version)")
        if ($Mode -ne "plain") {
            $d = Full $Dir
            if ($Mode -eq "collect") { New-Empty $d }
            else { "used.txt", "unknown.txt", "signtool-log.txt" | ForEach-Object { Remove-Item (Join-Path $d $_) -ErrorAction SilentlyContinue } }
            $shim = Join-Path $PSScriptRoot "signing\inno-signtool.ps1"
            # $q e $f são substituídos pelo ISCC (aspas e arquivo a assinar).
            $tool = 'pwsh -NoProfile -NonInteractive -File $q' + $shim + '$q -Mode ' + $Mode + ' -Dir $q' + $d + '$q -Target $f'
            $isccArgs += "/DSign=1", "/Ssigntool=$tool"
        }
        & (Find-Iscc) @isccArgs (Join-Path $root "installer\windows\SacraLink.iss") 2>&1 | Tee-Object -Variable isccOut | Out-Host
        $isccExit = $LASTEXITCODE
        if ($Mode -ne "plain" -and (Test-Path (Join-Path $d "signtool-log.txt"))) {
            Get-Content (Join-Path $d "signtool-log.txt") | ForEach-Object { Write-Annotation "notice" "SignTool: $_" }
        }
        if ($isccExit -ne 0) { throw "ISCC falhou: $(($isccOut | Select-Object -Last 6) -join ' | ')" }
        if ($Mode -eq "collect") {
            $n = @(Get-ChildItem -LiteralPath $d -Filter *.exe).Count
            if ($n -eq 0) { throw "o Inno Setup não pediu assinatura de nenhum arquivo interno" }
            Write-Host "Para assinar (rodada 2): $n arquivo(s) interno(s) do instalador"
        }
        if ($Mode -eq "replay") {
            $expected = @(Get-ChildItem -LiteralPath $d -Recurse -File -Filter *.exe | ForEach-Object { $_.BaseName.ToLower() })
            $used = @(if (Test-Path (Join-Path $d "used.txt")) { Get-Content (Join-Path $d "used.txt") })
            $missing = @($expected | Where-Object { $_ -notin $used })
            if (Test-Path (Join-Path $d "unknown.txt")) { Write-Host "Sem cópia assinada: $(Get-Content (Join-Path $d 'unknown.txt') -Raw)" }
            if ($missing.Count -gt 0 -or (Test-Path (Join-Path $d "unknown.txt"))) {
                throw "a segunda compilação não foi idêntica à da coleta: o instalador ficaria com partes sem assinatura"
            }
            Write-Host "Instalador recompilado com $($used.Count) arquivo(s) interno(s) assinado(s)"
        }
    }
    "stage-setup" {
        $d = Full $Dir; New-Empty $d
        Copy-Item (Join-Path $output $setupName) $d
    }
    "apply-setup" {
        Copy-Item (Find-In (Full $Dir) $setupName) (Join-Path $output $setupName) -Force
        Write-Host "Instalador assinado aplicado"
    }
    "verify" {
        $files = @($binaries | ForEach-Object { Join-Path $dist $_ }) + (Join-Path $output $setupName)
        foreach ($f in $files) {
            $s = Get-AuthenticodeSignature -LiteralPath $f
            if (-not $s.SignerCertificate) { throw "$(Split-Path -Leaf $f) está sem assinatura" }
            if ($RequireTrusted -and $s.Status -ne "Valid") { throw "$(Split-Path -Leaf $f): assinatura não confiável ($($s.Status))" }
            Write-Host ("{0,-26} {1} ({2})" -f (Split-Path -Leaf $f), $s.SignerCertificate.Subject, $s.Status)
        }
    }
    "checksums" {
        $h = (Get-FileHash (Join-Path $output $setupName) -Algorithm SHA256).Hash.ToLower()
        # Quebra de linha LF: "sha256sum -c" no Linux/macOS rejeita CRLF.
        [IO.File]::WriteAllText((Join-Path $output "SHA256SUMS.txt"), "$h  $setupName`n")
        Get-ChildItem $output | Format-Table Name, Length | Out-String | Write-Host
    }
    "test" {
        $work = Join-Path $root "signing-test"
        $sign = Join-Path $PSScriptRoot "signing\sign-test.ps1"
        Invoke-Self @{ Phase = "stage-binaries"; Dir = "$work\1" }
        & $sign -Path "$work\1"
        Invoke-Self @{ Phase = "apply-binaries"; Dir = "$work\1" }
        Invoke-Self @{ Phase = "compile"; Version = $Version; Mode = "collect"; Dir = "$work\2" }
        & $sign -Path "$work\2"
        Invoke-Self @{ Phase = "compile"; Version = $Version; Mode = "replay"; Dir = "$work\2" }
        Invoke-Self @{ Phase = "stage-setup"; Dir = "$work\3" }
        & $sign -Path "$work\3"
        Invoke-Self @{ Phase = "apply-setup"; Dir = "$work\3" }
        Invoke-Self @{ Phase = "verify" }
        Write-Host "Fluxo de assinatura validado (certificado de teste)" -ForegroundColor Green
    }
}
