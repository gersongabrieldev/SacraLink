# ADR-0003 — Proxy WHIP no servidor Go e MediaMTX restrito ao loopback

- Status: Aceito
- Data: 2026-11-05

## Contexto
O MediaMTX expõe WHIP/WHEP, RTSP, API de controle e métricas. A especificação
proíbe publicar endpoints internos do MediaMTX e exige autenticação por
dispositivo, tokens temporários e rate limiting.

## Decisão
- HTTP do WebRTC (`webrtcAddress`), RTSP, API e métricas do MediaMTX escutam em
  `127.0.0.1`.
- Somente a porta ICE (`webrtcLocalUDPAddress`/`webrtcLocalTCPAddress`, 8189) é
  exposta. Ela não aceita mídia sem uma sessão negociada pelo SDP.
- O celular envia o SDP offer para `https://<igreja>:7443/device/v1/whip`
  (TLS com fingerprint fixado no QR). O servidor Go autentica o dispositivo,
  verifica limites (8 transmissões) e repassa a oferta ao MediaMTX com uma
  credencial interna aleatória por instalação. A resposta (SDP answer +
  `Location`) é reescrita para o endereço do proxy. PATCH (trickle ICE) e DELETE
  (fim da sessão) também passam pelo proxy.
- O MediaMTX usa `authInternalUsers`: publicação só com a credencial interna e
  vinda de 127.0.0.1; leitura (OBS/painel) só de 127.0.0.1/::1.

## Consequências
- (+) Uma única porta HTTPS com nossa política de segurança.
- (+) O path de mídia é controlado pelo servidor (`sl/<cameraId>`); o celular não
  escolhe nomes arbitrários.
- (−) O proxy precisa propagar corretamente `Content-Type: application/sdp`,
  `Location`, `ETag` e `Link` (servidores ICE). Coberto por testes.
