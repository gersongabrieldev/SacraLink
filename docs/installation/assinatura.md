# Assinatura de código e Smart App Control

## Por que é obrigatória para distribuir

No Windows 11 com **Smart App Control** ligado (padrão em instalações novas),
executáveis **sem assinatura** e sem reputação podem ser bloqueados — sem
opção de "executar assim mesmo". Isso foi observado no próprio
desenvolvimento do SacraLink (2026-11-05): binários recém-compilados foram
bloqueados de forma intermitente (`An Application Control policy has blocked
this file`). O SmartScreen também alerta em instaladores não assinados.

Confirmado em 2026-11-21: o instalador da v0.2.0 foi bloqueado ao instalar
(`Erro 4551: Uma política de Controle de Aplicativo bloqueou este arquivo`,
no programa temporário `.tmp` que o Inno Setup extrai). Não há exceção por
arquivo no Smart App Control: a orientação da Microsoft é assinar.

Portanto, uma release pública deve ter **assinados todos os executáveis** que
o Windows carrega:

- `SacraLink-Setup-x64.exe`, **o programa temporário que ele extrai** e o
  desinstalador;
- `SacraLink.exe`, `sacralink-server.exe` e `sacralink-server.com`;
- `mediamtx.exe` (MIT, de terceiros): sem assinatura, o servidor não
  conseguiria iniciá-lo nas máquinas com Smart App Control.

Mesmo assinado, um programa novo pode ser bloqueado por alguns dias até ganhar
reputação (relatos de desenvolvedores, não documentação oficial).

## Opções de certificado (situação em 2026-11)

| Opção | Situação para o SacraLink |
|---|---|
| **SignPath Foundation** (escolhida) | Gratuita para código aberto: licença OSI (MIT atende), repositório público, build automático no CI, 2FA no GitHub e na SignPath. O editor do certificado aparece como "SignPath Foundation". Pedido em [signpath.org/apply](https://signpath.org/apply); política em [CODE_SIGNING_POLICY.md](../../CODE_SIGNING_POLICY.md) |
| Azure Trusted Signing (Artifact Signing) | Indisponível: pessoas físicas só nos EUA e Canadá; organizações nos EUA, Canadá, União Europeia e Reino Unido |
| Certificado OV/IV de uma CA comercial | Plano B pago (algumas centenas de dólares por ano). Desde 2023 a chave fica em token/HSM, então o CI precisa do serviço de assinatura em nuvem da CA |

## Plano com a SignPath Foundation

1. **Mantenedor:** ativar 2FA no GitHub, criar a conta na SignPath e enviar o
   pedido em [signpath.org/apply](https://signpath.org/apply), apontando para
   o repositório e a política.
2. **Depois da aprovação:** o mantenedor cria o token de API na SignPath e o
   cadastra como segredo do repositório (`SIGNPATH_API_TOKEN`, em *Settings →
   Secrets and variables → Actions*), e informa o ID da organização e os
   nomes do projeto e das políticas.
3. **CI:** o `mediamtx.exe` passa a ser compilado do código-fonte na versão
   fixada; o `release.yml` envia os executáveis à SignPath, monta o
   instalador com eles e envia o instalador (com o programa temporário e o
   desinstalador) para assinar; o mantenedor aprova cada pedido.

## Como o pipeline está preparado

- `installer/windows/SacraLink.iss` aceita `/DSign=1` e um `SignTool`
  definido na linha de comando do ISCC (`/Ssigntool=signtool sign /fd sha256
  /tr http://timestamp.digicert.com /td sha256 /a $f`).
- No GitHub Actions, adicione o certificado como segredo (ou use a ação do
  Azure Trusted Signing) e assine os `.exe` em `dist-release/SacraLink` antes
  de compilar o instalador.

Sem certificado, o instalador funciona em máquinas sem Smart App Control,
mas o usuário verá alertas do SmartScreen.
