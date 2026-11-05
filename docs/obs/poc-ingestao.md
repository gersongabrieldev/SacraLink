# PoC — Ingestão no OBS (RTSP vs WHEP)

Objetivo: escolher, com dados, o mecanismo padrão de ingestão das câmeras no OBS
(ADR-0004).

## Montagem

1. Servidor SacraLink rodando; uma câmera publicando (celular real ou publisher de
   teste `scripts/dev/publish-test-pattern.ps1`).
2. No OBS, duas fontes da mesma câmera:
   - `PoC A — RTSP` (Media Source, configuração do ADR-0004).
   - `PoC B — WHEP` (Browser Source `http://127.0.0.1:7380/player/<cameraId>`,
     "Controlar áudio pelo OBS" marcado, 1920×1080, 30 FPS).
3. Monitor com relógio em milissegundos (ex.: página `tests/performance/clock.html`
   em tela cheia em um segundo monitor).

## Medição de latência glass-to-glass

1. Aponte o celular para o relógio.
2. Coloque o relógio real e a fonte do OBS lado a lado em uma única captura
   (projetor do OBS em janela + relógio), ou fotografe ambos com outro celular a
   240 FPS.
3. Latência = relógio real − relógio exibido no OBS. Faça 20 amostras por modo,
   registre mediana e p95.

## Outras métricas

| Métrica | Ferramenta |
|---|---|
| CPU/GPU do OBS | Gerenciador de Tarefas (detalhes do `obs64.exe`) + `GetStats` do obs-websocket |
| Frames perdidos/renderização atrasada | `GetStats` (render/output skipped frames) |
| Sincronia A/V | Claquete (palmas) filmada; comparar pico de áudio vs quadro |
| Recuperação | Desligar o Wi-Fi do celular por 5 s; tempo até a imagem voltar no OBS |
| Estabilidade | 2 h contínuas, contar quedas |

## Registro

Resultados em `docs/test-reports/AAAA-MM-DD-poc-ingestao.md`, incluindo
modelo do celular, versão do OBS, CPU/GPU do PC, rede (banda, canal).
