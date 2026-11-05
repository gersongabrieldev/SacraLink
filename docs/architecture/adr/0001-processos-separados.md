# ADR-0001 — Servidor headless separado do painel

- Status: Aceito
- Data: 2026-11-05

## Contexto
A especificação exige que o vídeo continue sendo recebido com a janela minimizada,
que exista ícone na bandeja e que serviços possam ser reiniciados com segurança.
Wails v2 não oferece bandeja nativa, e um travamento do WebView2 derrubaria todo o
processo se backend e UI fossem o mesmo executável.

## Decisão
- `sacralink-server.exe`: processo Go headless com bandeja do Windows. Contém APIs,
  orquestração, ponte OBS, SQLite e supervisiona o `mediamtx.exe`.
- `SacraLink.exe`: painel Wails v2 (React). Conversa com o servidor somente pela
  Admin API em `127.0.0.1:7380` (REST + WebSocket), autenticada por senha do
  administrador. Se o servidor não estiver rodando, o painel o inicia.
- Fechar o painel não interrompe câmeras. "Sair" na bandeja encerra tudo, pedindo
  confirmação se houver câmeras transmitindo.

## Consequências
- (+) Isolamento de falhas, mesma API serve o painel e futuros clientes (tablet).
- (+) Testes de integração do servidor sem UI.
- (−) Dois executáveis para empacotar e uma API local a proteger (loopback + auth + CORS restrito).
