# Política de assinatura de código

> **Situação:** o SacraLink está pedindo a assinatura gratuita da
> [SignPath Foundation](https://signpath.org/) para projetos de código aberto.
> Enquanto o pedido não é aprovado, as releases saem **sem assinatura** e o
> Controle Inteligente de Aplicativos do Windows 11 pode bloquear o instalador
> ([detalhes](docs/installation/assinatura.md)). Depois da aprovação, as
> releases assinadas trarão: *Free code signing provided by
> [SignPath.io](https://about.signpath.io/), certificate by
> [SignPath Foundation](https://signpath.org/)*.

## O que é assinado

- `SacraLink-Setup-x64.exe`: o instalador, o programa temporário que ele
  extrai ao rodar e o desinstalador;
- `SacraLink.exe` (painel), `sacralink-server.exe` (servidor) e
  `sacralink-server.com` (comandos de terminal);
- `mediamtx.exe`: componente de terceiros
  ([MediaMTX](https://github.com/bluenviron/mediamtx), licença MIT). Para ser
  assinado junto com o restante, passa a ser compilado a partir do
  código-fonte oficial, na versão fixada, pelo próprio CI.

Só são assinados binários compilados pelo **GitHub Actions** a partir deste
repositório público, numa tag `vX.Y.Z` da branch `main`. Compilações locais
nunca são assinadas.

## Funções

| Função | Quem |
|---|---|
| Autores (commits) | [gersongabrieldev](https://github.com/gersongabrieldev) |
| Revisores | [gersongabrieldev](https://github.com/gersongabrieldev) |
| Aprovadores (liberam cada assinatura) | [gersongabrieldev](https://github.com/gersongabrieldev) |

Todos os membros usam autenticação em dois fatores no GitHub e na SignPath.
Contribuições externas entram por pull request revisado pelo mantenedor.

## Processo

1. A versão é criada enviando a tag `vX.Y.Z` a partir da `main`.
2. O workflow [`release.yml`](.github/workflows/release.yml) compila tudo do
   código-fonte ([`build-windows.ps1`](scripts/build-windows.ps1) e
   [`SacraLink.iss`](installer/windows/SacraLink.iss)).
3. O workflow envia os executáveis à SignPath e o mantenedor **aprova
   manualmente** cada pedido de assinatura.
4. Os arquivos assinados são publicados no
   [GitHub Releases](https://github.com/gersongabrieldev/SacraLink/releases)
   com o `SHA256SUMS.txt`.

## Privacidade

O SacraLink não transfere informações para outros sistemas na rede, exceto
quando isso é pedido por quem o instala ou opera:

- **Função principal:** vídeo, áudio e comandos entre os celulares, este
  computador e o OBS Studio, na rede da igreja (sem nuvem e sem relay).
- **Aviso de versão nova** (ligado por padrão; desligável em *Configurações →
  Avisar quando houver versão nova*): consulta a lista pública de releases em
  `api.github.com`, enviando apenas o cabeçalho `User-Agent: SacraLink/<versão>`.
  Nenhum dado da igreja, das câmeras ou dos usuários é enviado, e nada é
  baixado ou instalado sem o operador clicar.
- **Acesso externo 4G/5G** (desligado por padrão): quando o administrador
  habilita, o servidor consulta servidores STUN públicos (Google e Cloudflare)
  para descobrir o IP público da igreja.

---

## Code signing policy (English summary)

*Status: applying for free code signing by the SignPath Foundation. Until
approved, releases are unsigned.*

- **Signed files:** the installer (`SacraLink-Setup-x64.exe`, including the
  temporary setup program it extracts and the uninstaller), `SacraLink.exe`,
  `sacralink-server.exe`, `sacralink-server.com`, and `mediamtx.exe` (MIT,
  built from the official source at a pinned version by our CI).
- **Only** binaries built by GitHub Actions from this public repository, from
  a `vX.Y.Z` tag on `main`, are signed; local builds are never signed.
- **Roles:** committers, reviewers and approvers:
  [gersongabrieldev](https://github.com/gersongabrieldev). MFA is enabled on
  GitHub and SignPath. Each signing request is approved manually.
- **Privacy:** this program does not transfer information to other networked
  systems unless requested by the person installing or operating it, except
  for (1) its core function (video, audio and control between phones, this
  computer and OBS on the local network), (2) an update check against the
  public GitHub releases list (`api.github.com`, only the `User-Agent` header;
  can be turned off in Settings), and (3) public STUN servers, only when the
  administrator enables remote (4G/5G) access.
