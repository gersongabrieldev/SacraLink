# Configuração do OBS

## 1. Habilitar o WebSocket

OBS → **Ferramentas → Configurações do servidor WebSocket**:

- Marque *Habilitar servidor WebSocket* (porta padrão 4455).
- Mantenha *Habilitar autenticação* com uma senha.
- No SacraLink: *OBS e áudio → Importar do OBS deste PC* (lê endereço, porta
  e senha do `config.json` do obs-websocket; somente leitura).

A senha é guardada cifrada com DPAPI do Windows. O SacraLink **nunca** expõe
o obs-websocket na rede: a conexão é sempre `127.0.0.1`.

## 2. Studio Mode (recomendado)

Com o **Studio Mode** ligado, o SacraLink envia câmeras ao **Preview** e só
leva ao ar com *Cortar/Transição*. Sem Studio Mode, toda troca vai direto ao
ar e o painel pede confirmação (a menos que você permita trocas diretas em
*Segurança das trocas*).

## 3. O que o SacraLink cria no OBS

Com sua autorização, para cada câmera:

| Objeto | Nome | Conteúdo |
|---|---|---|
| Fonte | `SacraLink — Câmera 01` | Media Source lendo `rtsp://127.0.0.1:8554/sl/<id>` |
| Cena | `SacraLink — Cena Câmera 01` | A fonte ajustada ao canvas |

- Use as cenas dedicadas ou adicione a fonte às suas cenas (PiP, multicâmera):
  o **tally funciona em cenas aninhadas e grupos**.
- Renomear a câmera no SacraLink renomeia a fonte e a cena. Renomear no OBS
  também é aceito (o SacraLink acompanha).
- Se você apagar a fonte no OBS, o SacraLink **não** a recria sozinho
  (botão *Recriar fonte no OBS*).
- Fontes, cenas e configurações criadas por você **nunca** são alteradas ou
  apagadas.

## 4. Ingestão: RTSP (padrão) ou WebRTC

| Modo | Fonte no OBS | Observações |
|---|---|---|
| RTSP (padrão) | Media Source (FFmpeg) | Compatível com OBS 28+, áudio no mixer, reconexão automática; `rtsp_transport=tcp`, buffer mínimo, decodificação por hardware |
| WebRTC (experimental) | Browser Source `http://127.0.0.1:7380/player/<id>` | Potencialmente menor latência; marque "Controlar áudio pelo OBS" |

A escolha final será feita com as medições da [PoC de ingestão](poc-ingestao.md).
Troque por câmera em *Câmeras e aparelhos → Editar*.

## 5. Áudio

- O áudio do celular chega na própria fonte da câmera e aparece no mixer.
- O mixer oficial continua sendo o do OBS; o painel mostra níveis, mute,
  volume e permite definir áudio **principal** e **reserva**.
- Para sincronizar com a mesa de som, ajuste o *Atraso do áudio* da câmera
  (ms) em *Editar câmera* — vale para a fonte no OBS.
- Nunca use o alto-falante do celular que está captando (microfonia).

## 6. Tally (o que o celular mostra)

| Celular | Condição no OBS |
|---|---|
| **● NO AR** | A fonte está visível no Program (cena, cena aninhada ou grupo) |
| **PREVIEW** | Visível no Preview (Studio Mode) |
| **CONECTADA** | Transmitindo para o servidor, fora do Program/Preview |
| **OBS SEM CONEXÃO** | Estado desconhecido — nunca mostra NO AR antigo |

Limitações: opacidade 0, recorte total ou posição fora do canvas não são
detectados; plugins que injetam fontes fora das cenas (ex.: Downstream Keyer)
são cobertos pelo sinal "ativo" do próprio OBS.
