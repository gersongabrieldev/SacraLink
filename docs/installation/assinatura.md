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

- **MediaMTX do código-fonte:** `scripts/build-mediamtx.ps1` compila a versão
  fixada (tag conferida contra o commit) com a receita oficial do projeto;
  `build-windows.ps1 -MediaMTXFromSource` usa esse binário no CI e nas
  releases, para que ele possa ser assinado.
- **Três rodadas de assinatura** (`scripts/build-installer.ps1` e
  `.github/workflows/release.yml`):
  1. executáveis: `SacraLink.exe`, `sacralink-server.exe`,
     `sacralink-server.com` e `mediamtx.exe` (o `mediamtx.exe.sha256` é
     refeito depois, porque a assinatura muda o hash);
  2. partes internas do instalador: o motor `uninst.e32`, usado tanto no
     desinstalador quanto nas cópias temporárias que o instalador extrai ao
     rodar (o `.tmp` bloqueado na v0.2.0). O Inno Setup só o assina durante a
     compilação, chamando o SignTool, e confere se o arquivo voltou assinado.
     O SignTool do SacraLink (`scripts/signing/inno-signtool.ps1`) guarda a
     cópia sem assinatura numa primeira compilação (devolvendo ao ISCC uma
     assinatura descartável, só para ele seguir); depois de assinada pela
     SignPath, uma segunda compilação idêntica usa a cópia assinada,
     conferida pelo SHA-256. Se a compilação não for idêntica, o build falha
     em vez de publicar partes sem assinatura;
  3. o instalador final.
- **SignPath:** cada rodada envia uma pasta como artefato do GitHub Actions
  para a ação `signpath/github-action-submit-signing-request@v3`; o
  mantenedor aprova o pedido no painel da SignPath. A configuração de
  artefato fica em `.signpath/artifact-configuration.xml`. Fica ativa quando
  o repositório tem a variável `SIGNPATH_ORGANIZATION_ID` e o segredo
  `SIGNPATH_API_TOKEN`; sem eles, a release sai sem assinatura. A conferência
  final exige assinatura confiável no Windows em todos os executáveis.
- **Teste a cada envio:** o `ci.yml` roda as três rodadas com um certificado
  descartável (`scripts/signing/sign-test.ps1`), para que o fluxo já esteja
  validado quando a SignPath for configurada.
- Com um certificado próprio instalado na máquina, também é possível:
  `iscc /DSign=1 "/Ssigntool=signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a $f" ...`.

Sem certificado, o instalador funciona em máquinas sem Smart App Control,
mas o usuário verá alertas do SmartScreen.
