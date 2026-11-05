# ADR-0004 — Ingestão no OBS: RTSP (padrão) e WHEP (alternativa)

- Status: Aceito provisoriamente — decisão final após PoC medida
- Data: 2026-11-05

## Contexto
O OBS ≥ 30 publica WHIP, mas não há fonte de reprodução WHEP nativa comprovada
nas versões-alvo (28+). Opções reais:

- **A — RTSP / Media Source** (`ffmpeg_source`), lendo `rtsp://127.0.0.1:8554/sl/<id>`.
- **B — WHEP / Browser Source**, carregando uma página local servida pelo
  SacraLink com cliente WHEP em JavaScript.

## Decisão
Implementar as duas como estratégias da mesma interface (`SourceFactory`), com
**A como padrão** pelo suporte universal, áudio nativo no mixer e reconexão
automática do próprio OBS. A estratégia B é selecionável por câmera para
comparação.

Configuração inicial de A: `is_local_file=false`, `input_format=rtsp`,
`ffmpeg_options="rtsp_transport=tcp fflags=nobuffer"`, `buffering_mb=1`,
`reconnect_delay_sec=1`, `hw_decode=true`, `restart_on_activate=false`,
`close_when_inactive=false`.

## Critérios da PoC
Latência glass-to-glass (método do relógio em ms filmado e capturado no
Program), CPU/GPU do OBS com 1/4/8 fontes, estabilidade em 2 h, sincronia A/V,
tempo de recuperação após queda do celular. Ver `docs/obs/poc-ingestao.md`.
