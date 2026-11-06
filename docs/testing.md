# Testes do SacraLink

Este documento separa **o que foi realmente testado** (e onde) do que ainda
depende de **aparelhos reais**. Nada é declarado validado sem evidência.

## 1. Testes automatizados

Rodar tudo:

```bash
scripts/dev/test-server.sh                      # servidor Go (repete só bloqueios do Smart App Control)
cd apps/desktop/frontend && npm test            # painel
cd apps/mobile && flutter analyze && flutter test   # app (após scripts/mobile/bootstrap.sh)
```

| Área | Pacote / arquivo | O que cobre |
|---|---|---|
| Banco | `internal/store` | Migrations idempotentes, pareamento atômico, token de uso único e expiração, slots, reatribuição de câmera, retenção, bloqueio de login |
| Segurança | `internal/secure`, `tlsutil`, `ratelimit`, `devices` | IDs/tokens, bcrypt, certificado e **pinning** (fingerprint errado é recusado), bloqueio por IP, QR sem segredos, validação de entrada |
| Windows | `internal/platform` | DPAPI real, Job Object (filho morre com o pai), instância única, métricas, aviso do Windows 10 |
| OBS | `internal/obs` | Handshake com senha (vetor conferido de forma independente), senha errada/ausente, erros, eventos, queda do OBS, chave de stream não vaza |
| Tally | `internal/tally` (90% de cobertura) | Cenas aninhadas, grupos, itens ocultos, contêiner oculto, ciclos, Preview+Program, OBS desconectado nunca mostra NO AR |
| Ponte OBS | `internal/obsbridge` (12 testes) | Fonte e cena criadas uma única vez, renomeação no OBS seguida, fonte removida não recriada sem confirmação, adoção sem duplicar, nome de terceiro preservado, troca de modo RTSP↔WHEP, Studio Mode, confirmação de troca direta, reinício do OBS, senha errada |
| Mídia | `internal/media` | Config restrita ao loopback, conflito de portas, API v1.21, integridade SHA-256, **MediaMTX real** morto e reiniciado sozinho |
| Controle | `internal/control` | Hello/ack, tally na reconexão, comando com confirmação, validação, substituição de sessão sem travar, telemetria |
| Núcleo | `internal/core` | Máquina de estados, detecção de fluxo, criação de fonte, RECONNECTING, limite de transmissões, **fallback para câmera reserva**, Program intocado sem fallback |
| WHIP | `internal/whip` | Reescrita de Location, ETag/Link, isolamento entre câmeras, SDP inválido barrado |
| Rede | `internal/netdiag` | Classificação (CGNAT 100.64/10 etc.), STUN, orientações de CGNAT/DNS/portas |
| **Ponta a ponta** | `internal/app/e2e_test.go` | Setup → OBS → pareamento → controle → **WHIP com H.264 + Opus reais** → MediaMTX real → **RTSP lido como o OBS** → fonte criada → tally NO AR no celular → troca manual de cena → zoom confirmado → player → queda → credencial errada recusada |
| Painel | `src/lib/state.test.ts` | Reducer, formatação, grade |
| App | `test/*.dart` | QR Code, conflito/idempotência, expiração com relógio dessincronizado, adaptação de qualidade |

### Onde rodaram

| Ambiente | Resultado |
|---|---|
| Windows 11 Pro (build 26300), Ryzen 3 3300X, 16 GB — máquina do mantenedor | Todos os testes Go e do painel passaram (2026-11-05) |
| GitHub Actions `windows-latest` | Servidor (inclui ponta a ponta com MediaMTX real), painel e build Windows: **passaram** |
| GitHub Actions `ubuntu-latest` | Servidor com **detector de corridas** (`-race`) e ponta a ponta com MediaMTX real; app Flutter (`flutter analyze`, testes, **APK de depuração**): **passaram** |
| GitHub Actions `macos-latest` | App Flutter: **build iOS sem assinatura passou** |
| GitHub Actions (release manual) | **`SacraLink-Setup-x64.exe` gerado** (Inno Setup 6.7.1, 30 MB) como artefato privado; ainda não instalado/testado em VM |

Resultado medido no teste ponta a ponta (MediaMTX v1.21.3 real): o RTSP
entregue ao "OBS" trouxe **630 pacotes de vídeo e 100 de áudio em 2 s**
(H.264 + Opus), com bitrate medido no servidor de ~2,8 Mbps — coerente com o
vídeo sintético I_PCM de 160×96 a 15 FPS.

### Problemas reais encontrados pelo CI e corrigidos

- Condição de corrida ao seguir renomeações feitas no OBS (detectada pelo `-race`).
- MediaMTX não iniciava quando o SO recusava o buffer UDP de 8 MB (Linux); o supervisor agora volta ao padrão do sistema.
- Conflitos de dependências do app (`battery_plus` × `wakelock_plus`; `permission_handler` exigindo API 37).

### Validação manual já feita (sem aparelhos)

- `sacralink-server.exe` compilado executou no Windows 11 com Smart App
  Control ligado; recusou `Host` falso (HTTP 421); ao ser morto à força, o
  MediaMTX encerrou junto (Job Object).
- `SacraLink.exe` (Wails/WebView2) abriu a janela.
- Demonstração com OBS simulado e 3–4 câmeras sintéticas: painel exibiu a
  **prévia WHEP decodificando o H.264**, NO AR/PREVIEW, fontes e cenas
  renomeadas no OBS (capturas em `docs/img/`).

## 2. O que AINDA NÃO foi testado (requer hardware real)

| Item | Por quê |
|---|---|
| OBS Studio real | Os testes usam um OBS simulado com o mesmo protocolo; o OBS 32.0.1 instalado na máquina não foi alterado sem autorização |
| Android e iPhone reais | Sem aparelhos no ambiente; o app só foi analisado/compilado no CI |
| Latência glass-to-glass | Exige celular + OBS reais (metodologia abaixo) |
| Carga com 8 celulares | Exige aparelhos e hardware de referência ([performance.md](performance.md)) |
| Rede 4G/5G, troca Wi-Fi→5G, CGNAT, IPv6 | Exige rede externa real |
| Instalador | Gerado pelo CI de release; precisa ser testado em VM Windows 10/11 limpa |

## 3. Teste automatizado com o OBS Studio real

Com o OBS aberto (WebSocket habilitado) no computador de teste:

```bash
cd apps/server
SACRALINK_REAL_OBS=1 go test -run TestOBSReal -v ./internal/app/
```

Salvaguardas: aborta se o OBS estiver transmitindo ou gravando; cria uma
coleção de cenas nova ("SacraLink — Teste …") sem tocar nas cenas existentes;
volta para a coleção original no final. Verifica: conexão autenticada com a
configuração importada do obs-websocket, criação da fonte, **captura da fonte
feita pelo próprio OBS com imagem não vazia** (prova de decodificação do
H.264), tally NO AR vindo do Program real, fonte ativa (`GetSourceActive`) e
presença no mixer de áudio. A captura é salva em `%TEMP%\sacralink-obs-real-*.png`.

## 4. Roteiro de testes manuais

Registre cada execução em `docs/test-reports/AAAA-MM-DD-<tema>.md` com
aparelho, versão do SO, versão do OBS, CPU/GPU e rede.

### 4.1 Funcionais (§19.1)

1. **Instalação:** VM Windows 11 limpa → `SacraLink-Setup-x64.exe` → marcar
   firewall → concluir. Esperado: atalho, bandeja, assistente.
2. **OBS:** OBS aberto com WebSocket habilitado (Ferramentas → Configurações
   do WebSocket). No assistente: *Importar configuração do OBS deste PC*.
   Esperado: "OBS conectado".
3. **Pareamento Android/iPhone:** *Parear celular* → ler QR. Esperado: câmera
   "Câmera 01" na grade; aparelho em *Câmeras e aparelhos*.
4. **Vídeo e áudio:** *Iniciar transmissão* no celular. Esperado: card em
   TRANSMITINDO com bitrate; fonte `SacraLink — Câmera 01` no OBS com imagem;
   medidor de áudio no mixer do OBS.
5. **Troca de cena:** com Studio Mode, *Preview* → borda verde no card e
   PREVIEW no celular; *Cortar* → NO AR no celular em < 0,5 s.
6. **Cena aninhada:** criar cena "Missa" contendo a cena da câmera; colocar
   "Missa" no ar pelo OBS. Esperado: NO AR no celular.
7. **Zoom remoto:** mover o controle no painel. Esperado: zoom muda no
   celular; o celular informa o valor real.
8. **Conflito:** mexer no zoom no celular e, em seguida, no painel. Esperado:
   aviso "o operador do celular alterou agora".
9. **Reconexão:** desligar o Wi-Fi do celular por 5 s. Esperado:
   RECONECTANDO → volta sozinho; a fonte do OBS volta sem duplicar.
10. **Persistência:** reiniciar o PC. Esperado: câmeras, fontes e senha mantidas.

### 4.2 OBS (§19.4)

OBS fechado (tally "OBS sem conexão"), aberto sem transmitir, Studio Mode,
transmitindo, gravando, reinício do OBS (reconecta e sincroniza), remoção
manual da fonte (não recria sem *Recriar fonte*), fonte em cena aninhada.

### 4.3 Rede (§19.3)

Wi-Fi 5 GHz, Wi-Fi congestionado, servidor no cabo, 4G, 5G, troca Wi-Fi→5G
(nova sessão WHIP automática), perda momentânea, IPv4 público com
encaminhamento de portas, IPv6, CGNAT sem acesso de entrada (o diagnóstico
deve explicar a inviabilidade). Use *Testar conexão externa* no app com o
Wi-Fi desligado.

### 4.4 Latência glass-to-glass

1. Abra `tests/performance/clock.html` em tela cheia (relógio em ms).
2. Aponte o celular para o relógio e coloque a fonte do OBS em projetor ao
   lado do relógio real.
3. Fotografe ambos (câmera de 240 FPS ideal); latência = real − exibido.
4. 20 amostras por configuração (RTSP e WHEP; Wi-Fi e 4G); registre mediana
   e p95. Metas iniciais: < 500 ms em Wi-Fi; ≤ 2 s em 4G/5G.
