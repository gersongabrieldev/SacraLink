<p align="center">
  <img src="assets/logo.svg" alt="SacraLink" width="110"/>
</p>

<h1 align="center">SacraLink</h1>
<p align="center"><b>Sistema de Transmissão Sem Fio para Igrejas</b><br/>
Transforme até 8 celulares Android/iPhone em câmeras sem fio para o OBS Studio —
sem VPS, sem relay em nuvem, sem Docker.</p>

<p align="center">
  <a href="https://github.com/gersongabrieldev/SacraLink/actions/workflows/ci.yml"><img src="https://github.com/gersongabrieldev/SacraLink/actions/workflows/ci.yml/badge.svg" alt="CI"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/licen%C3%A7a-MIT-blue" alt="MIT"/></a>
  <a href="README.en.md">English</a> ·
  <a href="docs/architecture/00-plano-tecnico.md">Plano técnico</a> ·
  <a href="docs/mvp-checklist.md">Status do MVP</a>
</p>

> **Status: pré-MVP em desenvolvimento ativo.** O fluxo de vídeo
> celular → servidor → OBS está implementado e coberto por testes ponta a ponta
> com o MediaMTX real; **a validação com celulares reais e com o OBS Studio
> real ainda está pendente**. Veja o [checklist do MVP](docs/mvp-checklist.md)
> e o [registro de testes](docs/testing.md).

<p align="center">
  <img src="docs/img/painel-producao.png" alt="Painel de produção do SacraLink com 4 câmeras, uma NO AR e uma em PREVIEW" width="900"/><br/>
  <sub>Painel com 4 câmeras sintéticas de teste (demonstração local com OBS simulado).</sub>
</p>

## O problema

Paróquias que transmitem missas raramente têm orçamento para câmeras sem fio
profissionais. Os voluntários já têm celulares capazes, mas as ferramentas de
"celular como webcam" costumam ser de câmera única, dependentes de nuvem ou
não sabem o que está realmente no ar no OBS.

## O que o SacraLink faz

- **Celulares viram câmeras sem instalar nada**: aponte a câmera do celular
  para o QR Code e a **câmera abre no navegador** (Chrome/Safari), enviando
  H.264 + Opus por **WebRTC (WHIP)** direto ao computador da igreja. O app
  nativo SacraLink Camera é opcional (4G/5G, lentes reais, pinning).
- **Integração real com o OBS** (obs-websocket 5.x): cria uma fonte e uma cena
  por câmera, sem duplicar em reconexões e sem tocar no que é seu.
- **Tally de verdade**: o celular mostra **● NO AR / PREVIEW / CONECTADA**
  conforme o Program/Preview real do OBS — inclusive em **cenas aninhadas e
  grupos** — e nunca mostra NO AR desatualizado se o OBS cair.
- **Produção segura**: Preview → Corte com Studio Mode; trocas diretas e
  início/fim de transmissão exigem confirmação; fallback opcional para câmera
  reserva quando a câmera no ar cai.
- **Controle remoto**: zoom, lente (somente as reais do aparelho), foco,
  lanterna, microfone e qualidade, com confirmação de execução e política de
  conflito com o operador do celular.
- **Sem VPS**: funciona em rede local sem internet; 4G/5G funciona quando há
  rota de entrada real (IPv4 público ou IPv6). CGNAT é detectado e explicado.
- **Windows nativo**: instalador `.exe`, servidor na bandeja, painel desktop,
  sem Docker, WSL, Node.js ou Python no computador da igreja.

## Arquitetura

```
Celulares (navegador ou app) ──HTTPS/WSS :7443──▶ sacralink-server.exe ──▶ MediaMTX (somente loopback)
        └──────── mídia WebRTC UDP/TCP :8189 ─────────────────▶ MediaMTX
SacraLink.exe (painel Wails) ──127.0.0.1:7380──▶ sacralink-server.exe ──ws 127.0.0.1:4455──▶ OBS
Media Source do OBS ──rtsp://127.0.0.1:8554──▶ MediaMTX   (sem transcodificação)
```

| Camada | Tecnologias |
|---|---|
| Servidor | Go 1.27, SQLite (Go puro), supervisor do MediaMTX v1.21, cliente obs-websocket 5.x próprio |
| Painel | Wails v2, React 19, TypeScript, Tailwind CSS 4 |
| Câmera no navegador | HTML/CSS/JavaScript puro servido pelo próprio servidor (`/cam/`): getUserMedia + WebRTC/WHIP, sem build e sem dependências |
| App móvel (opcional) | Flutter 3.47, flutter_webrtc, plugin nativo (Kotlin/Swift) para lentes e zoom reais |
| Instalador | Inno Setup 6 (`SacraLink-Setup-x64.exe`) |

Decisões e justificativas: [plano técnico](docs/architecture/00-plano-tecnico.md)
e [ADRs](docs/architecture/adr/).

## Segurança

- Painel só em `127.0.0.1`, com senha (bcrypt), bloqueio por tentativas e
  proteção contra DNS rebinding/CSRF.
- Celulares pareados por **QR Code com token de uso único (5 min)** — sem
  senha no QR, com os dados no fragmento do link (nunca enviado ao servidor) —
  e **TLS 1.3**; o app nativo faz **pinning** do certificado, o navegador
  depende do aceite do certificado do computador da igreja
  ([ADR-0006](docs/architecture/adr/0006-camera-pelo-navegador.md)).
- Credenciais dos aparelhos revogáveis, guardadas só como hash; senha do OBS
  cifrada com DPAPI; MediaMTX e obs-websocket nunca expostos na rede.
- Acesso externo desligado por padrão. Detalhes em [SECURITY.md](SECURITY.md).

## Documentação

| Para | Leia |
|---|---|
| Instalar na igreja | [Guia de instalação](docs/installation/guia-instalacao.md) |
| Configurar o OBS | [Configuração do OBS](docs/obs/configuracao-obs.md) |
| 4G/5G sem VPS | [Acesso externo](docs/networking/acesso-externo.md) |
| Usar o celular como câmera | [Câmera pelo navegador](docs/mobile/camera-navegador.md) · [App SacraLink Camera](docs/mobile/README.md) |
| APIs e protocolo | [APIs](docs/api/README.md) · [Protocolo de controle](packages/contracts/control-v1.md) |
| Testes e capacidade | [Testes](docs/testing.md) · [Desempenho](docs/performance.md) |
| Desenvolver | [Ambiente de desenvolvimento](docs/development.md) · [Contribuir](CONTRIBUTING.md) |

## Requisitos (usuário final)

- Windows 11 ou Windows 10 1809+ 64 bits (o Windows 10 está fora de suporte — o app avisa)
- OBS Studio 28+ com o servidor WebSocket habilitado
- Celulares no mesmo Wi-Fi: pelo navegador, Chrome atualizado (Android) ou
  Safari do iOS 15.5+; pelo app, Android 7+ ou iOS 15.5+ (o app também
  funciona pela internet)

## Limitações conhecidas

- Acesso por 4G/5G é impossível atrás de CGNAT sem IPv6 — por projeto, não há relay.
- A câmera para em segundo plano ou com a tela bloqueada (restrição de Android/iOS).
- Câmera pelo navegador: aviso de certificado na primeira vez, sem 4G/5G
  nesta versão e recursos (zoom, lanterna) conforme o navegador; ainda não
  testada em celulares reais ([detalhes](docs/mobile/camera-navegador.md)).
- Zoom óptico e lentes dependem do que cada aparelho expõe.
- Suporte a 8 câmeras é meta de projeto, ainda não validada com aparelhos
  ([desempenho](docs/performance.md)).
- Executáveis não assinados podem ser bloqueados pelo Smart App Control
  ([assinatura](docs/installation/assinatura.md)).
- O pareamento é sempre pelo QR Code; a descoberta na rede (mDNS) só reencontra servidores já pareados.

## Roadmap

Fases 0–6 no [plano técnico](docs/architecture/00-plano-tecnico.md#13-roadmap-técnico-priorizado).
Próximos passos: validação com OBS e aparelhos reais, PoC de latência
RTSP × WebRTC, primeira release assinada, testes de carga com 8 celulares.

## Licença

[MIT](LICENSE). Componentes de terceiros: [docs/licenses.md](docs/licenses.md).
