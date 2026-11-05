# Licenças de terceiros

Revisão inicial de dependências (atualizar a cada nova dependência).

| Componente | Uso | Licença | Redistribuído no instalador? | Observações |
|---|---|---|---|---|
| MediaMTX | Servidor de mídia | MIT | Sim (`mediamtx.exe` + `LICENSE`) | Binário oficial de release, hash verificado no build |
| Go standard library | Servidor | BSD-3-Clause | Compilado no binário | |
| modernc.org/sqlite | Banco | BSD-3-Clause | Compilado | Sem CGO |
| golang.org/x/sys, x/time, x/crypto | Windows APIs, rate limit | BSD-3-Clause | Compilado | |
| coder/websocket | WebSocket | ISC | Compilado | |
| fyne.io/systray | Bandeja | Apache-2.0 | Compilado | |
| skip2/go-qrcode | QR Code | MIT | Compilado | |
| Wails v2 | Painel desktop | MIT | Compilado | Requer WebView2 Runtime (Microsoft, redistribuível) |
| React, React DOM | Painel | MIT | Empacotado (JS) | |
| Tailwind CSS | Painel | MIT | Empacotado (CSS) | |
| Flutter / Dart SDK | App móvel | BSD-3-Clause | No app | |
| flutter_webrtc | App móvel | MIT | No app | Inclui libwebrtc (BSD-3) |
| mobile_scanner | App móvel | BSD-3-Clause | No app | Android usa Google ML Kit (termos do Google) |
| OBS Studio | Produção | GPL-2.0 | **Não** | Comunicação apenas por protocolo WebSocket, sem linkagem |
| Inno Setup | Instalador | Licença Inno Setup (permite uso comercial) | Ferramenta de build | |
