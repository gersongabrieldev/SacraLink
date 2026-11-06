# Protocolo de controle v1 (celular ↔ servidor)

Implementação de referência: `apps/server/internal/control` (Go) e
`apps/mobile/lib/src/control_channel.dart` + `session.dart` (Dart).

## Transporte

- `wss://<servidor>:7443/device/v1/ws`, subprotocolo `sacralink.control.v1`.
- TLS 1.3 com **pinning**: o app aceita apenas o certificado cuja impressão
  digital SHA-256 veio no QR Code.
- Autenticação: cabeçalho `Authorization: Bearer <deviceId>.<segredo>`.
- Mensagens de texto JSON, até 64 KiB.

## Envelope

```json
{ "v": 1, "type": "telemetry", "id": "opcional", "ts": 1793906400000, "data": { } }
```

`v` diferente de 1 encerra a conexão.

## Ciclo de vida

1. O aparelho conecta e envia `hello` em até 10 s (senão a conexão é fechada).
2. O servidor responde `hello_ack` e, se houver, o último `tally` conhecido.
3. O aparelho envia `ping` a cada `heartbeatSec` (5 s). Sem mensagens por
   15 s, o servidor encerra a sessão.
4. Uma nova conexão da mesma câmera **substitui** a anterior (troca de rede).

## Mensagens do aparelho

| Tipo | Dados |
|---|---|
| `hello` | `appVersion`, `platform` (`android`/`ios`), `model`, `osVersion`, `capabilities` |
| `telemetry` | `battery {level 0..1, charging}`, `thermal` (`nominal`/`fair`/`serious`/`critical`), `stream {state, bitrateKbps, fps, width, height, packetsLost, rttMs, jitterMs, availableKbps}`, `camera {lensId, facing, zoom, torch, micMuted}`, `network {type, quality}` — no máximo 10/s |
| `command_result` | `commandId`, `status`, `message?`, `state?` |
| `local_change` | `param`, `value` — o operador do celular alterou um controle |
| `ping` | — |

`capabilities` informa somente o que o aparelho **realmente** suporta:

```json
{
  "lenses": [
    { "id": "0", "facing": "back", "kind": "wide", "label": "Principal", "minZoom": 1, "maxZoom": 10 },
    { "id": "2", "facing": "back", "kind": "ultra_wide", "label": "Ultra-angular", "minZoom": 1, "maxZoom": 2 }
  ],
  "torch": true, "focusPoint": true, "exposurePoint": true,
  "maxWidth": 1920, "maxHeight": 1080, "maxFps": 60, "hardwareH264": true, "thermal": true
}
```

`opticalZoomFactors` só aparece quando a plataforma informa fatores ópticos
reais (ex.: câmera virtual do iPhone). Sem ele, o painel não promete zoom óptico.

## Mensagens do servidor

| Tipo | Dados |
|---|---|
| `hello_ack` | `sessionId`, `deviceId`, `cameraId`, `cameraName`, `slot`, `serverName`, `whipPath`, `profile`, `heartbeatSec` |
| `tally` | `state` (`on_air`/`preview`/`connected`/`unknown`), `program`, `preview`, `live`, `obsConnected`, `seq` |
| `command` | ver abaixo |
| `config` | `cameraName?`, `profile?` |
| `pong` | — |

### Tally

- `on_air`: a fonte da câmera está visível no **Program** do OBS (inclusive
  em cenas aninhadas e grupos) ou o OBS a marca como ativa.
- `preview`: visível no Preview (somente com Studio Mode).
- `connected`: transmitindo, mas fora do Program/Preview.
- `unknown`: OBS desconectado — o app **nunca** mostra NO AR nesse estado.
- `live`: o OBS está transmitindo ou gravando.

## Comandos

```json
{
  "commandId": "k3x...", "sessionId": "s7a...", "name": "zoom.set",
  "params": { "value": 2.5 }, "issuedAt": 1793906400000, "expiresAt": 1793906405000, "force": false
}
```

| Nome | Parâmetros |
|---|---|
| `zoom.set` | `value` (0,1–100; o aparelho limita à faixa real) |
| `zoom.step` | `delta` (−10–10) |
| `lens.select` | `lensId` (de `capabilities.lenses`) |
| `camera.flip` | — |
| `focus.point`, `exposure.point` | `x`, `y` (0–1) |
| `torch.set` | `on` |
| `audio.mute` | `muted` |
| `quality.set` | `profileId` + valores resolvidos pelo servidor (`width`, `height`, `fps`, `maxBitrateKbps`, `minBitrateKbps`, `adaptive`) |
| `stream.reconnect` | — |
| `preset.apply` | `presetId` + ajustes resolvidos pelo servidor: `lensId?`, `zoom?`, `profile?` (perfil completo), `torch?`, `micMuted?` |

O servidor valida nome e parâmetros (lista de permissão) **antes** de enviar.

### Confirmação (`command_result.status`)

| Status | Significado |
|---|---|
| `executed` | aplicado; `state` traz os valores reais (ex.: zoom efetivo) |
| `rejected` | recusado (ex.: `sessionId` de outra sessão) |
| `unsupported` | o aparelho não tem o recurso (ex.: sem lanterna) |
| `failed` | erro ao aplicar |
| `conflict` | o operador do celular alterou o mesmo controle há menos de 3 s |
| `expired` | chegou depois da validade |
| `timeout` | (servidor) o aparelho não respondeu em 5 s |
| `offline` | (servidor) aparelho sem conexão de controle |

### Regras de execução no aparelho

1. **Idempotência:** o mesmo `commandId` devolve o resultado já calculado.
2. **Sessão:** `sessionId` diferente da sessão atual → `rejected`.
3. **Expiração robusta a relógio:** validade = `expiresAt − issuedAt`, contada
   a partir do recebimento (os relógios do celular e do PC podem divergir).
4. **Conflito:** controles locais sempre funcionam e dão ao operador do
   celular prioridade de 3 s sobre aquele parâmetro; `force: true` (confirmado
   pelo operador central) ignora a prioridade. Um preset altera vários
   controles e é recusado se qualquer um deles estiver com prioridade local.
5. **Capacidades reais:** comandos para recursos inexistentes → `unsupported`.
