# Histórico de mudanças

Todas as mudanças relevantes do projeto são registradas aqui. O formato segue o
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o projeto usa
[Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [Não lançado]

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
