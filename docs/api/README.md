# APIs do SacraLink

O servidor expõe **duas APIs separadas** (especificação §17):

| API | Endereço | Quem usa | Autenticação |
|---|---|---|---|
| Administração | `http://127.0.0.1:7380` (somente loopback) | Painel, ferramentas locais | Senha do administrador → token de sessão |
| Dispositivos | `https://<ip>:7443` (TLS 1.3) | Câmera pelo navegador (`/cam/`) e app SacraLink Camera | Token de pareamento → credencial do aparelho |

O protocolo de controle em tempo real está em
[`packages/contracts/control-v1.md`](../../packages/contracts/control-v1.md).

## Convenções

- JSON UTF-8; campos desconhecidos são **rejeitados** (`400 invalid_request`).
- Erros: `{"error": {"code": "…", "message": "…"}}` com código estável.
- Ações que podem afetar a transmissão exigem confirmação explícita
  (`"confirm": true` ou `?confirm=true`); sem ela a resposta é
  `409 confirmation_required`.
- Operações de leitura e de "garantir estado" (ex.: criar fonte no OBS) são
  idempotentes. Comandos para celulares têm `commandId` único e confirmação.

### Códigos de erro

| Código | HTTP | Situação |
|---|---|---|
| `invalid_request` | 400 | JSON inválido ou campo desconhecido |
| `unauthorized` | 401 | Sem sessão / credencial inválida |
| `login_failed` | 401 | Senha incorreta |
| `rate_limited`, `account_locked` | 429 | Excesso de tentativas |
| `invalid_host`, `invalid_origin` | 421/403 | Proteção contra DNS rebinding/CSRF |
| `not_found` | 404 | Recurso inexistente |
| `confirmation_required` | 409 | Ação exige confirmação |
| `studio_mode_off` | 409 | Preview sem Studio Mode no OBS |
| `no_source`, `source_removed`, `ingest_mode_mismatch` | 409 | Estado da fonte no OBS |
| `obs_disconnected` | 503 | OBS sem conexão |
| `stream_limit` | 503 | Limite de transmissões simultâneas |
| `media_starting` | 503 | Servidor de mídia iniciando (`Retry-After`) |
| `device_revoked`, `device_blocked` | 403 | Aparelho desautorizado |
| `pairing_invalid` | 401 | Token de pareamento inválido/expirado/usado |
| `too_many_presets` | 409 | Câmera com 20 presets |

## Admin API (`/api/v1`)

Proteções: escuta só em 127.0.0.1; `Host` obrigatório `127.0.0.1:7380`/
`localhost:7380`; `Origin` restrito ao painel (Wails/servidor/Vite em dev);
`Authorization: Bearer <token>` (sessão de 12 h, deslizante).

| Método e caminho | Descrição |
|---|---|
| `GET /health` | Saúde e versão (público) |
| `GET /setup/status` | Administrador configurado? Primeira execução concluída? (público) |
| `POST /setup/admin` `{password, serverName}` | Cria o administrador (só uma vez) → `{token}` |
| `POST /setup/complete` | Marca a primeira execução como concluída |
| `POST /auth/login` `{password}` | → `{token}`; 5 falhas bloqueiam por 5 min |
| `POST /auth/logout` · `POST /auth/password` `{current, new}` | Sessão |
| `POST /auth/ws-ticket` | Ticket de uso único (30 s) para o WebSocket |
| `GET /ws?ticket=…` | WebSocket de eventos: `snapshot`, `camera`, `obs`, `system`, `media`, `event`, `audio`, `settings`, `command`, `device_paired` |
| `GET /system/status` · `GET /system/logs` | Estado, versões, Windows, métricas, logs do MediaMTX |
| `POST /system/media/restart` | Reinicia o MediaMTX |
| `POST /system/selftest` | Teste de funcionamento (banco, mídia, TLS dos celulares, OBS, firewall, rede, Windows) — somente leitura |
| `GET /system/firewall` · `POST /system/firewall {confirm}` | Regras do firewall (eleva via UAC) |
| `POST /system/autostart {enabled}` | Início com o Windows |
| `GET /settings` · `PUT /settings` | Configurações |
| `GET /network/status` · `POST /network/diagnose` | Diagnóstico de rede |
| `GET /devices` | Aparelhos |
| `POST /devices/pair {label?, cameraId?}` | QR de pareamento → `{uri, qrPng, expiresAt}` |
| `PATCH /devices/{id} {name}` | Renomear |
| `POST /devices/{id}/revoke\|block\|unblock\|disconnect` | Autorização |
| `DELETE /devices/{id}?confirm=true` | Remover (a câmera permanece) |
| `GET /profiles` | Perfis de qualidade |
| `GET /cameras` · `GET /cameras/{id}` · `PATCH /cameras/{id}` | Câmeras (`name`, `operatorName`, `profileId`, `ingestMode`, `isBackup`, `audioSyncOffsetMs`, `orientation`: `landscape` ou `portrait` — enviada na hora ao celular) |
| `POST /cameras/{id}/commands {name, params, force}` | Comando remoto → resultado confirmado |
| `POST /cameras/{id}/zoom\|lens\|quality` | Atalhos da especificação |
| `GET /cameras/{id}/metrics?since=` | Métricas (amostras de 10 s) |
| `GET /cameras/{id}/presets` · `POST /cameras/{id}/presets {name, fromCurrent | params}` · `DELETE /cameras/{id}/presets/{pid}` | Presets (até 20 por câmera; `fromCurrent` usa a telemetria do celular) |
| `POST /cameras/{id}/presets/{pid}/apply {force?}` | Aplica o preset no celular → resultado confirmado |
| `GET /obs/status` · `GET /obs/detect` | Estado / instalação local do OBS |
| `GET /obs/config` · `PUT /obs/config` · `POST /obs/config/import` | Conexão com o obs-websocket |
| `GET /obs/scenes` | Cenas e câmeras visíveis em cada uma |
| `POST /obs/preview {cameraId \| sceneName}` | Enviar ao Preview |
| `POST /obs/program {cameraId \| sceneName, confirm}` | Levar ao ar (Studio Mode: Preview + transição) |
| `POST /obs/transition` | Preview → Program |
| `GET /obs/transitions` · `PUT /obs/transitions/current {name}` | Transições |
| `GET /obs/outputs` · `POST /obs/outputs/stream\|record {action, confirm}` | Transmissão/gravação |
| `POST /obs/sources/sync` · `POST /obs/cameras/{id}/source {recreate}` | Fontes do SacraLink |
| `GET /obs/audio` · `PATCH /obs/audio/{input} {muted, volumeDb, syncOffsetMs}` | Áudio |
| `GET /streams` · `GET /metrics` · `GET /events` · `GET /audit` · `GET /sessions` | Diagnóstico |
| `GET /player/{cameraId}` | Página WHEP para Browser Source |
| `POST /whep/{cameraId}` (+ `PATCH`/`DELETE` da sessão) | Prévia WebRTC (loopback) |

## Device API (`/device/v1`)

| Método e caminho | Descrição |
|---|---|
| `GET /info` | Identificação do servidor (sem dados sensíveis) |
| `POST /pair {token, device}` | → `{deviceId, secret, cameraId, cameraName, slot, serverId, serverName}` (o segredo só é mostrado aqui) |
| `GET /ws` | Canal de controle (ver contrato). Autenticação pelo cabeçalho `Authorization` (app) ou `?ticket=` (navegador) |
| `POST /ws-ticket` | → `{ticket, expiresInSec}`: ticket de **uso único (30 s)** para o WebSocket do navegador, que não envia cabeçalhos |
| `GET /cam/` (fora de `/device/v1`) | Câmera pelo navegador: página estática com CSP estrita; `GET /` redireciona para cá |
| `POST /whip` (`application/sdp`) | Oferta WHIP → `201` + SDP de resposta + `Location` |
| `PATCH /whip/sessions/{id}` | Trickle ICE / ICE restart |
| `DELETE /whip/sessions/{id}` | Encerrar transmissão |
| `POST /reachability {network}` | Teste de acesso externo (feito pelo celular fora do Wi-Fi) |

Limites: 20 req/s por IP; pareamento 1 a cada 5 s por IP; 10 falhas de
autenticação em 5 min bloqueiam o IP por 10 min.
