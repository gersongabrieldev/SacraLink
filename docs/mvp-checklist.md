# Checklist do MVP (especificação §20)

Legenda:
- ⬜ não iniciado
- 🟨 implementado (código existe; falta validação)
- 🟩 testado automaticamente (teste no repositório, executado localmente e/ou no CI)
- ✅ validado manualmente com o componente real (registro em `docs/test-reports/`)

**O MVP NÃO está concluído**: os critérios que dependem de celulares reais e
do OBS Studio real ainda precisam de validação manual (ver
[testing.md](testing.md)).

| # | Critério | Status | Evidência / próximo passo |
|---|---|---|---|
| 1 | Instalação por `.exe` no Windows | 🟨 | `SacraLink-Setup-x64.exe` **gerado pelo CI** (artefato privado, 30 MB); falta instalar em VMs Windows 10/11 limpas e assinar |
| 2 | Serviços iniciam sem Docker/terminal | 🟩 | `sacralink-server.exe` (subsistema GUI, bandeja) executado no Windows 11; supervisor do MediaMTX testado com o binário real |
| 3 | App mostra estado de funcionamento | 🟩 | Cabeçalho do painel validado visualmente (servidor, OBS, câmeras, IP, CPU/RAM, rede, AO VIVO) |
| 4 | Android transmite vídeo real por Wi-Fi | 🟨 | App analisado, testado e **APK de depuração gerado no CI**; **falta teste em aparelho** |
| 5 | iPhone transmite vídeo real por Wi-Fi | 🟨 | **Build iOS sem assinatura passou no CI**; falta conta Apple Developer e teste em aparelho |
| 6 | Servidor recebe os fluxos | 🟩 | Teste ponta a ponta: WHIP com H.264 + Opus reais aceito pelo MediaMTX real (CI Windows e Linux) |
| 7 | Vídeo reproduzido no OBS em fonte utilizável | 🟨 | RTSP verificado como o OBS consome (DESCRIBE/SETUP/PLAY, 630 pacotes de vídeo + 100 de áudio em 2 s); prévia WHEP decodificada no painel. **Falta validar no OBS Studio real** |
| 8 | Áudio recebido e controlável | 🟨 | Opus no fluxo e no RTSP (teste); mute/volume/offset via obs-websocket (OBS simulado). **Falta OBS real** |
| 9 | Fonte cadastrada/associada automaticamente | 🟩 | 12 testes da ponte OBS (idempotência, renomeação, adoção, remoção manual) contra OBS simulado; **falta OBS real** |
| 10 | Conexão OBS autenticada | 🟩 | Autenticação SHA-256 testada (vetor independente), senha errada/ausente; DPAPI real |
| 11 | Painel identifica a câmera conectada | 🟩 | Teste ponta a ponta + demonstração visual com câmeras sintéticas |
| 12 | Painel coloca a câmera em Preview/Program | 🟩 | Testes com Studio Mode, confirmação de troca direta, transição; **falta OBS real** |
| 13 | Celular recebe NO AR corretamente | 🟩 | Teste ponta a ponta (NO AR → CONECTADA ao trocar cena no OBS); cenas aninhadas e grupos no motor de tally; **falta aparelho real** |
| 14 | Zoom remoto em aparelho compatível | 🟨 | Protocolo, validação, conflito e confirmação testados com celular simulado; zoom nativo via flutter_webrtc + faixa real via plugin; **falta aparelho real** |
| 15 | Logs e tratamento de falhas | 🟩 | Logs JSON rotativos; reinício automático do MediaMTX; fallback de buffer UDP; reconexão do OBS; mensagens amigáveis |
| 16 | Documentação de instalação e teste | 🟩 | [Instalação](installation/guia-instalacao.md), [OBS](obs/configuracao-obs.md), [rede externa](networking/acesso-externo.md), [testes](testing.md), [app](mobile/README.md) |

## Pendências conhecidas (não bloqueiam o código, mas bloqueiam o "MVP concluído")

- Validação com OBS Studio real, Android e iPhone (itens 4, 5, 7, 8, 13, 14).
- Primeira release com instalador e teste em VM.
- PoC de latência RTSP × WHEP ([poc-ingestao.md](obs/poc-ingestao.md)).
- Descoberta automática por mDNS ("Localizar servidor na rede"): não
  implementada; o pareamento usa QR Code ou código manual.
- Presets de câmera (`camera_presets` existe no banco; telas e API pendentes).
- Assinatura de código ([assinatura.md](installation/assinatura.md)).
