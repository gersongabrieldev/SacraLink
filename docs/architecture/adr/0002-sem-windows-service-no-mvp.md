# ADR-0002 — Sem Windows Service no MVP

- Status: Aceito
- Data: 2026-11-05

## Contexto
Foi solicitada a análise de Windows Services para o servidor de mídia.

## Análise
| Critério | Windows Service | Processo do usuário (autostart no logon) |
|---|---|---|
| OBS disponível | OBS exige sessão interativa; o serviço não traz ganho | Mesmo ciclo de vida do OBS |
| Privilégio | LocalSystem/LocalService, exposto à rede | Usuário comum (menor superfície) |
| Bandeja / notificações | Impossível (Sessão 0) | Nativo |
| DPAPI da senha do OBS | Escopo de máquina ou conta de serviço | Escopo do usuário |
| Início antes do logon | Sim | Não (irrelevante: sem logon não há OBS) |
| Instalação | Exige admin e registro de serviço | Chave HKCU\...\Run |

## Decisão
O servidor roda como processo do usuário com início automático opcional no logon.
O MediaMTX é processo filho, associado a um *Job Object* com
`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, para nunca ficar órfão. O supervisor reinicia
o MediaMTX com backoff se ele cair.

## Consequências
Um modo serviço pode ser adicionado no futuro (ex.: PC dedicado sem OBS, recebendo
para gravação isolada), reaproveitando o mesmo binário com `--service`.
