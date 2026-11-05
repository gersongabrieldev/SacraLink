<p align="center">
  <img src="assets/logo.svg" alt="SacraLink" width="120"/>
</p>

<h1 align="center">SacraLink</h1>
<p align="center"><b>Sistema de Transmissão Sem Fio para Igrejas</b><br/>
Transforme até 8 celulares Android/iPhone em câmeras sem fio para o OBS Studio —
sem VPS, sem relay em nuvem, sem Docker.</p>

<p align="center">
  <a href="README.en.md">English</a> ·
  <a href="docs/architecture/00-plano-tecnico.md">Plano técnico</a> ·
  <a href="docs/mvp-checklist.md">Status do MVP</a>
</p>

> **Status: em desenvolvimento ativo (pré-MVP).** Veja o
> [checklist do MVP](docs/mvp-checklist.md) para saber o que está implementado, o
> que tem teste automatizado e o que ainda precisa de validação manual em
> aparelhos reais.

## O problema

Paróquias que transmitem missas raramente têm orçamento para sistemas
profissionais de câmeras sem fio. Os voluntários já têm celulares capazes, mas as
ferramentas de "celular como webcam" costumam ser de câmera única, dependentes de
nuvem ou não sabem o que está realmente no ar no OBS.

## A solução

O SacraLink roda no mesmo computador Windows que executa o OBS:

- Os celulares enviam H.264 + Opus por **WebRTC (WHIP)** diretamente ao PC da igreja.
- Um servidor de mídia **MediaMTX** local (gerenciado pelo SacraLink) converte os
  fluxos para **RTSP** para o OBS — sem transcodificação.
- O SacraLink cria e mantém uma fonte + cena no OBS por câmera via
  **obs-websocket 5.x**.
- Calcula o **tally** (PREVIEW / NO AR) a partir do estado real do OBS — incluindo
  cenas aninhadas e grupos — e envia para cada celular em tempo real.
- O operador controla **zoom, lente e qualidade** de cada celular remotamente.
- Funciona em rede local **sem internet**. 4G/5G é suportado quando a rede da
  igreja tem rota de entrada real (IPv4 público + redirecionamento de porta ou
  IPv6 global). CGNAT sem IPv6 é detectado e explicado com honestidade.

## Arquitetura

```
Celulares (Flutter) ──HTTPS/WSS :7443──▶ sacralink-server.exe ──▶ MediaMTX (loopback)
        └──────── mídia WebRTC UDP/TCP :8189 ─────────────────▶ MediaMTX
SacraLink.exe (painel Wails) ──127.0.0.1:7380──▶ sacralink-server.exe ──ws:4455──▶ OBS
Media Source do OBS ──rtsp://127.0.0.1:8554──▶ MediaMTX
```

| Camada | Tecnologias |
|---|---|
| Servidor | Go 1.27, SQLite (Go puro), supervisor do MediaMTX, cliente obs-websocket 5.x |
| Painel | Wails v2, React 18, TypeScript, Tailwind CSS |
| App móvel | Flutter, flutter_webrtc |
| Instalador | Inno Setup 6 (`SacraLink-Setup-x64.exe`) |

Detalhes no [plano técnico](docs/architecture/00-plano-tecnico.md) e nos
[ADRs](docs/architecture/adr/).

## Requisitos (usuário final)

- Windows 10/11 64 bits (o Windows 10 já saiu do suporte — o app avisa)
- OBS Studio 28 ou superior com o servidor WebSocket habilitado
- Microsoft Edge WebView2 Runtime (o instalador cuida disso)
- Celulares Android 8+ ou iOS 14+ no mesmo Wi-Fi (ou alcançáveis pela internet)

## Desenvolvimento

Veja [docs/development.md](docs/development.md).

## Segurança

Veja [SECURITY.md](SECURITY.md). O acesso remoto vem desligado; a API de
administração só escuta em 127.0.0.1; os celulares são pareados com tokens de uso
único e curta validade.

## Roadmap

Veja a seção 13 do [plano técnico](docs/architecture/00-plano-tecnico.md#13-roadmap-técnico-priorizado).

## Limitações conhecidas

- Acesso remoto (4G/5G) é impossível atrás de CGNAT sem IPv6 — por projeto, o
  SacraLink não usa relay.
- A câmera para quando o app vai para segundo plano ou a tela bloqueia
  (restrição do sistema, principalmente iOS). O app mantém a tela ligada.
- Zoom óptico e troca de lentes dependem das APIs de cada aparelho.
- Suporte a 8 câmeras é meta de projeto; só será declarado validado para o
  hardware e perfis de referência em [docs/performance.md](docs/performance.md)
  após os testes de carga.

## Licença

[MIT](LICENSE). Componentes de terceiros: [docs/licenses.md](docs/licenses.md).
