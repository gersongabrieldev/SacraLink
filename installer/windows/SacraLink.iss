; ============================================================================
; SacraLink — instalador Windows (Inno Setup 6)
; Gera: SacraLink-Setup-x64.exe
;
; Compilação (após scripts\build-windows.ps1):
;   iscc /DAppVersion=0.1.0 installer\windows\SacraLink.iss
;
; Assinatura de código (recomendada; exige certificado do mantenedor):
;   iscc /DAppVersion=0.1.0 /DSign=1 "/Ssigntool=signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a $f" ...
; ============================================================================

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#define AppName "SacraLink"
#define AppPublisher "SacraLink"
#define AppURL "https://github.com/gersongabrieldev/SacraLink"
#define SourceDir "..\..\dist-release\SacraLink"

[Setup]
AppId={{6B1F4C2E-5A7D-4E31-9C0B-3D2A8F7E1C55}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
DefaultDirName={autopf}\SacraLink
DefaultGroupName=SacraLink
DisableProgramGroupPage=yes
LicenseFile=..\..\LICENSE
OutputDir=Output
OutputBaseFilename=SacraLink-Setup-x64
SetupIconFile=..\..\assets\icons\installer.ico
UninstallDisplayIcon={app}\SacraLink.exe
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
; Windows 10 1809+ e Windows 11, somente 64 bits.
MinVersion=10.0.17763
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
; Program Files e regras de firewall exigem administrador.
PrivilegesRequired=admin
CloseApplications=force
RestartApplications=no
VersionInfoVersion={#AppVersion}
VersionInfoProductName={#AppName}
VersionInfoDescription=Instalador do SacraLink
#ifdef Sign
SignTool=signtool
SignedUninstaller=yes
#endif

[Languages]
Name: "ptbr"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
ptbr.TaskFirewall=Configurar o Firewall do Windows para os celulares (recomendado)
en.TaskFirewall=Configure Windows Firewall for the phones (recommended)
ptbr.TaskDesktop=Criar atalho na área de trabalho
en.TaskDesktop=Create a desktop shortcut
ptbr.Win10Warning=O Windows 10 encerrou o suporte regular em 14/10/2025 e não recebe mais atualizações de segurança gratuitas. O SacraLink funciona, mas expor este computador à internet (acesso remoto 4G/5G) exige cuidados adicionais. Recomendamos o Windows 11.
en.Win10Warning=Windows 10 reached end of support on 2025-10-14. SacraLink works, but exposing this PC to the internet (remote 4G/5G access) requires extra care. Windows 11 is recommended.
ptbr.RemoveData=Remover também os dados do SacraLink (câmeras pareadas, configurações, certificados e logs)?%n%nEscolha "Não" para manter os dados para uma reinstalação.
en.RemoveData=Also remove SacraLink data (paired cameras, settings, certificates and logs)?%n%nChoose "No" to keep them for a reinstall.
ptbr.WebView2Install=Instalando o Microsoft Edge WebView2 Runtime...
en.WebView2Install=Installing Microsoft Edge WebView2 Runtime...
ptbr.LaunchApp=Abrir o SacraLink
en.LaunchApp=Launch SacraLink

[Tasks]
Name: "firewall"; Description: "{cm:TaskFirewall}"; Flags: checkedonce
Name: "desktopicon"; Description: "{cm:TaskDesktop}"; Flags: unchecked

[Files]
Source: "{#SourceDir}\SacraLink.exe"; DestDir: "{app}"; Flags: ignoreversion signonce
Source: "{#SourceDir}\sacralink-server.exe"; DestDir: "{app}"; Flags: ignoreversion signonce
Source: "{#SourceDir}\mediamtx.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\mediamtx.exe.sha256"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\mediamtx-LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\sacralink.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\dashboard\*"; DestDir: "{app}\dashboard"; Flags: ignoreversion recursesubdirs createallsubdirs
; Bootstrapper oficial do WebView2 (baixado pelo CI; instalado só se faltar).
Source: "{#SourceDir}\..\MicrosoftEdgeWebview2Setup.exe"; DestDir: "{tmp}"; Flags: deleteafterinstall skipifsourcedoesntexist; Check: NeedsWebView2

[Icons]
Name: "{autoprograms}\SacraLink"; Filename: "{app}\SacraLink.exe"; Comment: "Painel de produção do SacraLink"
Name: "{autodesktop}\SacraLink"; Filename: "{app}\SacraLink.exe"; Tasks: desktopicon

[Run]
Filename: "{tmp}\MicrosoftEdgeWebview2Setup.exe"; Parameters: "/silent /install"; StatusMsg: "{cm:WebView2Install}"; Flags: waituntilterminated skipifdoesntexist; Check: NeedsWebView2
Filename: "{app}\sacralink-server.exe"; Parameters: "firewall apply --server-exe ""{app}\sacralink-server.exe"" --mediamtx-exe ""{app}\mediamtx.exe"" --ice-tcp"; Flags: runhidden waituntilterminated; Tasks: firewall
Filename: "{app}\SacraLink.exe"; Description: "{cm:LaunchApp}"; Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM SacraLink.exe"; Flags: runhidden; RunOnceId: "KillPainel"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM sacralink-server.exe"; Flags: runhidden; RunOnceId: "KillServidor"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM mediamtx.exe"; Flags: runhidden; RunOnceId: "KillMidia"
Filename: "{app}\sacralink-server.exe"; Parameters: "firewall remove"; Flags: runhidden waituntilterminated; RunOnceId: "FirewallRemove"

[Registry]
; Remove o início automático na desinstalação (criado pelo próprio app).
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: none; ValueName: "SacraLink"; Flags: uninsdeletevalue dontcreatekey

[Code]
function NeedsWebView2(): Boolean;
var
  Version: String;
begin
  Result := not (
    RegQueryStringValue(HKLM, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) or
    RegQueryStringValue(HKCU, 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version)
  ) or (Version = '') or (Version = '0.0.0.0');
end;

function InitializeSetup(): Boolean;
var
  V: TWindowsVersion;
begin
  GetWindowsVersionEx(V);
  if (V.Major = 10) and (V.Build < 22000) then
    MsgBox(CustomMessage('Win10Warning'), mbInformation, MB_OK);
  Result := True;
end;

// Encerra o SacraLink em execução antes de atualizar (os dados do usuário em
// %LOCALAPPDATA%\SacraLink são preservados).
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM SacraLink.exe', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM sacralink-server.exe', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM mediamtx.exe', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Result := '';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usPostUninstall) and not UninstallSilent then
    if MsgBox(CustomMessage('RemoveData'), mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
      DelTree(ExpandConstant('{localappdata}\SacraLink'), True, True, True);
end;
