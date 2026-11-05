# Assinatura de código e Smart App Control

## Por que é obrigatória para distribuir

No Windows 11 com **Smart App Control** ligado (padrão em instalações novas),
executáveis **sem assinatura** e sem reputação podem ser bloqueados — sem
opção de "executar assim mesmo". Isso foi observado no próprio
desenvolvimento do SacraLink (2026-11-05): binários recém-compilados foram
bloqueados de forma intermitente (`An Application Control policy has blocked
this file`). O SmartScreen também alerta em instaladores não assinados.

Portanto, uma release pública deve ter **assinados**:

- `SacraLink-Setup-x64.exe` (instalador e desinstalador);
- `SacraLink.exe` e `sacralink-server.exe`;
- `mediamtx.exe` (binário MIT de terceiros, redistribuído — pode ser
  assinado pelo distribuidor; a integridade também é verificada por SHA-256).

## Opções de certificado

| Opção | Observações |
|---|---|
| Certificado OV/EV de code signing (CA comercial) | Exige empresa/pessoa verificada; EV dá reputação imediata ao SmartScreen |
| Azure Trusted Signing | Serviço da Microsoft com custo mensal baixo; integra com `signtool` |
| SignPath Foundation | Assinatura gratuita para projetos open source elegíveis |

## Como o pipeline está preparado

- `installer/windows/SacraLink.iss` aceita `/DSign=1` e um `SignTool`
  definido na linha de comando do ISCC (`/Ssigntool=signtool sign /fd sha256
  /tr http://timestamp.digicert.com /td sha256 /a $f`).
- No GitHub Actions, adicione o certificado como segredo (ou use a ação do
  Azure Trusted Signing) e assine os `.exe` em `dist-release/SacraLink` antes
  de compilar o instalador.

Sem certificado, o instalador funciona em máquinas sem Smart App Control,
mas o usuário verá alertas do SmartScreen.
