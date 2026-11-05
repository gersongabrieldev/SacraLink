# Desempenho e capacidade

> **Status: suporte a 8 câmeras ainda NÃO validado.** O SacraLink foi
> projetado para até 8 transmissões simultâneas (limite configurável), mas a
> especificação (§19.2) proíbe declarar suporte validado sem testes com
> aparelhos reais no hardware de referência. Esta página define o método e
> registrará os resultados.

## Hardware de referência (a confirmar nos testes)

| Item | Mínimo proposto | Recomendado proposto |
|---|---|---|
| CPU | 4 núcleos (ex.: Ryzen 3 3300X / Core i3-10100) | 6+ núcleos |
| RAM | 8 GB | 16 GB |
| GPU | Decodificação H.264 por hardware | Idem + NVENC/AMF/QSV para o OBS |
| Rede do PC | Ethernet 1 Gbit até o roteador | Idem |
| Wi-Fi | 802.11ac 5 GHz | 802.11ax (Wi-Fi 6), canal dedicado |

## Perfis

| Perfil | Resolução | FPS | Bitrate máx. | 8 câmeras ≈ |
|---|---|---|---|---|
| Econômico | 480p | 30 | 1 Mbps | 8 Mbps |
| Equilibrado | 720p | 30 | 2,5 Mbps | 20 Mbps |
| Alta | 1080p | 30 | 4,5 Mbps | 36 Mbps |
| Alta 60 | 1080p | 60 | 6,5 Mbps | 52 Mbps |

O servidor **não transcodifica**: o custo de CPU do SacraLink/MediaMTX é
baixo (remux). O custo dominante está no **OBS** (decodificação de N fontes +
composição + codificação da saída) e na **prévia do painel** (decodificação
por hardware no WebView2; use "somente a selecionada" em máquinas modestas).

## Metodologia (progressiva: 1 → 2 → 4 → 6 → 8 câmeras)

1. Celulares reais no perfil em teste, por 30 min em cada etapa.
2. Coleta a cada 10 s:
   - SacraLink: `GET /api/v1/metrics` e `/api/v1/cameras` (bitrate, perdas,
     jitter, estado), CPU/RAM do sistema (cabeçalho do painel);
   - OBS: `GetStats` (CPU, FPS, quadros perdidos de renderização e saída);
   - Gerenciador de Tarefas: CPU/GPU de `obs64.exe`, `sacralink-server.exe`,
     `mediamtx.exe`, `msedgewebview2.exe`.
3. Critério de aprovação: sem quedas, perda < 1%, OBS sem quadros perdidos
   de renderização, latência dentro da meta.

### Pré-teste sem celulares (`sl-testpublisher`)

Gera N câmeras sintéticas (H.264 I_PCM + Opus) para validar o servidor, o
MediaMTX e o OBS antes dos aparelhos. **Não representa** o custo de um
codificador real (o vídeo sintético é sem compressão e muito mais pesado):

```bash
sl-testpublisher --admin-password "<senha>" --count 8 --duration 30m --width 160 --height 96
```

Observação registrada em 2026-11-05 (máquina de desenvolvimento, loopback):
3–4 câmeras sintéticas 320×176 a ~10 Mbps cada geraram perda de RTP em
rajadas no UDP com o buffer padrão do Windows. O buffer de leitura UDP do
MediaMTX foi aumentado para 8 MB. Reavaliar com aparelhos reais.

## Resultados

| Data | Hardware | Perfil | Câmeras | CPU total | OBS FPS | Perdas | Latência | Resultado |
|---|---|---|---|---|---|---|---|---|
| — | — | — | — | — | — | — | — | *pendente* |
