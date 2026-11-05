# Licenças de terceiros

Revisão de dependências (atualizar a cada nova dependência). O SacraLink é
distribuído sob a licença MIT; todas as dependências abaixo são compatíveis.

## Servidor e painel (Windows)

| Componente | Uso | Licença | Redistribuído? |
|---|---|---|---|
| MediaMTX v1.21.3 | Servidor de mídia | MIT | Sim (`mediamtx.exe` + `mediamtx-LICENSE.txt`, SHA-256 verificado) |
| Biblioteca padrão do Go | Servidor | BSD-3-Clause | Compilado |
| modernc.org/sqlite (+ libc, mathutil, memory) | Banco SQLite sem CGO | BSD-3-Clause | Compilado |
| golang.org/x/sys, x/crypto, x/time | APIs do Windows, bcrypt, rate limit | BSD-3-Clause | Compilado |
| github.com/coder/websocket | WebSocket (OBS, painel, controle) | ISC | Compilado |
| fyne.io/systray | Ícone na bandeja | Apache-2.0 | Compilado |
| github.com/skip2/go-qrcode | QR Code de pareamento | MIT | Compilado |
| gopkg.in/yaml.v3 | Config do MediaMTX | MIT / Apache-2.0 | Compilado |
| gopkg.in/natefinch/lumberjack.v2 | Rotação de logs | MIT | Compilado |
| github.com/wailsapp/wails/v2 | Painel desktop | MIT | Compilado (requer WebView2 Runtime, redistribuível da Microsoft) |
| github.com/tc-hib/winres | Ícone/manifesto/versão (build) | 0BSD | Ferramenta de build |
| React, React DOM | Painel | MIT | Empacotado |
| Tailwind CSS | Painel | MIT | Empacotado |
| lucide-react | Ícones do painel | ISC | Empacotado |

## Somente testes e desenvolvimento

| Componente | Uso | Licença |
|---|---|---|
| github.com/pion/webrtc/v4 (+ pion/*) | Celular simulado (`testpub`, `sl-testpublisher`) | MIT |
| Vite, TypeScript, Vitest | Build e testes do painel | MIT / Apache-2.0 |
| Inno Setup | Instalador | Licença Inno Setup (uso comercial permitido) |

## App móvel

| Componente | Licença | Observação |
|---|---|---|
| Flutter / Dart SDK | BSD-3-Clause | |
| flutter_webrtc | MIT | Inclui libwebrtc (BSD-3-Clause) |
| mobile_scanner | BSD-3-Clause | No Android usa Google ML Kit (termos do Google) |
| flutter_secure_storage | BSD-3-Clause | |
| wakelock_plus, connectivity_plus, device_info_plus, package_info_plus | BSD-3-Clause | |
| permission_handler | MIT | |
| crypto | BSD-3-Clause | |

## Não redistribuídos

| Componente | Licença | Relação |
|---|---|---|
| OBS Studio | GPL-2.0 | Apenas comunicação pelo protocolo obs-websocket; sem linkagem |
| FFmpeg | LGPL/GPL | Não usado nem distribuído (o OBS traz o seu) |
