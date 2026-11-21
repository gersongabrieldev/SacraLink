# Histórico de mudanças

Todas as mudanças relevantes do projeto são registradas aqui. O formato segue o
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o projeto usa
[Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [Não lançado]

## [0.2.0] - 2026-11-21

### Adicionado
- **Encerramento limpo ao atualizar ou desinstalar**: o instalador e o
  desinstalador pedem ao servidor que encerre do jeito normal (MediaMTX, OBS,
  banco e logs fechados corretamente) pelo evento `Local\SacraLinkServer.Quit`
  e só forçam (`taskkill`) o que sobrar após 20 s. Novo comando
  `sacralink-server quit [--timeout 20s]`. Vale a partir da próxima
  atualização (a 0.1.0 em execução ainda é encerrada à força).
- **Exposição e cor travadas pelo painel** (*Mais ações → Exposição e cor*):
  travar a exposição, ajustar o brilho (EV), travar a cor ou escolher a
  temperatura (2700–6500 K) e **copiar para as outras câmeras**, para que as
  imagens não oscilem com vitrais, velas e projetor e combinem no corte.
  Novos comandos `exposure.lock`, `exposure.compensation`,
  `whitebalance.lock` e `whitebalance.temperature`; capacidades
  `exposureLock`, `exposureCompensation`, `whiteBalanceLock` e
  `colorTemperature`. Funciona na câmera pelo navegador (Chrome no Android,
  quando a câmera permite); o app nativo ainda não informa esses recursos e
  o painel não os oferece para ele. Teste ponta a ponta com o Edge/Chrome
  headless (trava de exposição executada e confirmada; cor "não suportado"
  na câmera simulada). Ainda não testado em celulares reais.
- Documentação: **celular no cabo** com adaptador USB → Ethernet para a câmera
  principal, sem instalar nada nem ativar o modo desenvolvedor.
- CI compila o instalador (Inno Setup) a cada envio, não só na release.
- **Aviso de versão nova**: ao abrir o painel (e a cada 12 h no servidor), o
  SacraLink consulta a lista pública de releases no GitHub e mostra no topo
  *Nova versão do SacraLink* com **Baixar instalador** e **O que mudou**. Nada
  é baixado ou instalado sem o operador clicar; só links do repositório
  oficial são abertos. Quem está numa versão 0.x recebe pré-lançamentos; numa
  versão estável, só versões estáveis. Pode ser desligado em
  *Configurações* (`updateCheck`). Novos endpoints `GET /api/v1/update` e
  `POST /api/v1/update/check`.

### Corrigido
- **Comandos de terminal sem resposta**: `sacralink-server admin reset
  --confirm` (recuperação de senha), `version` e `firewall` não mostravam
  nada no PowerShell/cmd, porque o servidor é um aplicativo de janela. O
  instalador passa a incluir o `sacralink-server.com`, de console: chamado
  sem extensão (`.\sacralink-server admin reset --confirm`), ele mostra a
  saída, espera o término e devolve o código de saída. Chamado com `.exe`, o
  servidor se conecta ao console de quem o chamou e também mostra a resposta.
- `SHA256SUMS.txt` da release com quebra de linha LF (`sha256sum -c` no
  Linux/macOS rejeitava CRLF).
- Tela de login e guia de instalação com o comando de recuperação de senha
  que funciona no PowerShell (o caminho entre aspas com argumentos dava erro).

## [0.1.0] - 2026-11-21

Primeira versão pública, como **pré-lançamento**: instalador sem assinatura
digital e MVP ainda sem validação com o OBS Studio real e com celulares reais
([checklist](docs/mvp-checklist.md)).

### Adicionado
- **Orientação controlada pelo painel**: cada câmera pode ser horizontal
  (16:9) ou vertical (9:16). O celular recebe na hora (`hello_ack`/`config`):
  o app trava a tela na orientação, a página do navegador pede para girar e
  trava em tela cheia; o painel avisa "celular na posição errada".
- **Tela de permissões na câmera pelo navegador**: lista câmera, microfone,
  tela ligada e tela cheia/orientação, pede tudo num toque, explica como
  liberar o que foi recusado e segue sem zoom remoto ou sem som quando só
  isso foi negado.
- **Câmera pelo navegador** (`https://<ip>:7443/cam/`): o celular vira câmera
  sem instalar aplicativo. O QR Code passa a ser um link `https://` com os
  dados no fragmento; a página (JavaScript puro, CSP estrita) usa o mesmo
  pareamento, canal de controle v1 (por ticket de uso único) e WHIP do app.
  Teste ponta a ponta com Edge/Chrome reais em modo headless no CI
  ([ADR-0006](docs/architecture/adr/0006-camera-pelo-navegador.md)).
- Plano técnico, ADRs e estrutura do monorepo (Fase 0).
- Servidor `sacralink-server.exe`: SQLite com migrations, pareamento por QR
  com token de uso único, TLS 1.3 com pinning, Admin API (loopback) e Device
  API, canal de controle v1, proxy WHIP/WHEP, supervisor do MediaMTX
  (Job Object, reinício automático, fallback de buffer UDP), cliente
  obs-websocket 5.x, ponte OBS (fontes automáticas idempotentes, tally com
  cenas aninhadas/grupos, Preview/Program seguros, saídas, áudio), núcleo de
  câmeras (estados, limite de 8, fallback), diagnóstico de rede (STUN, CGNAT),
  bandeja do Windows, firewall com UAC, autostart, logs rotativos.
- Painel `SacraLink.exe` (Wails + React + TypeScript + Tailwind): assistente
  de primeira execução, produção multicâmera com prévia WHEP, câmeras e
  aparelhos, OBS e áudio, rede, configurações, eventos e auditoria.
- App SacraLink Camera (Flutter): pareamento, WHIP, tally, controles locais e
  remotos, adaptação de qualidade, reconexão na troca de rede, plugin nativo
  de lentes/zoom/temperatura/bateria.
- Instalador Inno Setup, build Windows reproduzível e CI (Linux com -race,
  Windows, painel, Android, iOS) e workflow de release.
- Ferramentas: celular simulado (`sl-testpublisher`), OBS simulado
  (`sl-fakeobs`), demonstração local e capturas de tela.
- Documentação: APIs, protocolo, instalação, OBS, acesso externo, app,
  testes, desempenho, assinatura de código.

[Não lançado]: https://github.com/gersongabrieldev/SacraLink/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/gersongabrieldev/SacraLink/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gersongabrieldev/SacraLink/releases/tag/v0.1.0
