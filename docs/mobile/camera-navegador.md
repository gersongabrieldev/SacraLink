# Câmera pelo navegador (sem instalar aplicativo)

O celular vira câmera do SacraLink direto no navegador: o próprio computador
da igreja serve a página `https://<ip-do-computador>:7443/cam/`. Não há
loja de aplicativos, conta da Apple ou APK para instalar. O app nativo
[SacraLink Camera](README.md) continua disponível e lê o mesmo QR Code.

## Como conectar um celular

1. No painel do SacraLink: **Câmeras e aparelhos → Parear celular**.
2. No celular, abra a **câmera** (ou o Google Lens) e aponte para o QR Code.
   Toque no link que aparecer.
3. **Primeira vez neste celular:** o navegador avisa que a conexão não é
   particular. Isso acontece porque o certificado é do próprio computador da
   igreja (não há domínio na internet). Confira se o endereço é o IP do
   computador da igreja e:
   - **Android (Chrome):** *Avançado* → *Continuar para 192.168.… (não seguro)*.
   - **iPhone (Safari):** *Mostrar Detalhes* → *visitar este site* → *Visitar Site*.
4. Digite um nome (ex.: "Pedro — altar") e toque em **Conectar**.
5. A tela **Permissões do celular** lista tudo o que será pedido. Toque em
   **Permitir acesso** e responda **Permitir** nos avisos do navegador
   (detalhes abaixo).
6. Deixe o celular na posição escolhida no computador (horizontal ou
   vertical) e toque em **Iniciar transmissão**.

## Permissões pedidas

| Permissão | Para quê | Se for recusada |
|---|---|---|
| **Câmera** | A imagem enviada ao OBS | Não dá para transmitir: a página mostra como liberar |
| **Microfone** | O som do celular | A página explica como liberar ou permite **continuar sem som** |
| **Mover a câmera** (Chrome) | Zoom controlado pelo operador central | A câmera funciona, só sem zoom remoto |
| **Tela sempre ligada** | Com a tela apagada o celular pausa a câmera | Automática (sem aviso); em navegadores antigos, desative o bloqueio automático da tela |
| **Tela cheia e orientação** | Trava a tela na posição escolhida no computador | Pedida ao tocar em *Iniciar transmissão*; no iPhone, gire à mão |

A página pede tudo de uma vez, num único toque, e segue com o que foi
liberado. Se o celular já liberou o site antes, ela vai direto para a câmera.
Ao voltar das configurações do celular, a lista se atualiza sozinha.

**Para liberar depois de recusar:**
- **Android (Chrome):** ícone ao lado do endereço (cadeado ou ⓘ) →
  *Permissões* → Câmera/Microfone → *Permitir*. Se continuar bloqueado:
  Configurações do Android → *Apps → Chrome → Permissões*.
- **iPhone (Safari):** *aA* na barra de endereço → *Configurações do Site* →
  Câmera/Microfone → *Permitir*. Ou: Ajustes → *Apps → Safari*.

## Horizontal ou vertical: quem decide é o computador

No painel do SacraLink, cada câmera tem a opção **Orientação**: *Horizontal
(16:9)* ou *Vertical (9:16)*. Ela fica no menu **⋯** do cartão da câmera
(mudança na hora) e em **Câmeras e aparelhos → Editar**.

- O celular recebe a escolha na hora. Se estiver na posição errada, aparece
  no centro da tela **"Gire o celular na VERTICAL (em pé)"** (ou HORIZONTAL).
- Em tela cheia, a página trava a tela na posição escolhida (Android). No
  iPhone, o Safari não permite travar: basta girar o aparelho.
- O painel compara com a imagem que está chegando e mostra **"Celular na
  posição errada"** no cartão da câmera.
- No OBS, a fonte se ajusta ao quadro sem distorcer: vídeo vertical numa cena
  horizontal fica com faixas nas laterais (e vice-versa).

O QR Code vale **5 minutos e um único celular**. Se expirar, gere outro.

### Nas próximas vezes

Abra o mesmo endereço (`https://<ip>:7443/cam/`) — o celular continua
pareado. Dica: use *Adicionar à tela inicial* do navegador para ter um atalho.

> Se o IP do computador mudar, o navegador trata o novo endereço como outro
> site e será preciso parear de novo. **Reserve o IP do computador no
> roteador** (reserva DHCP). O app nativo não tem essa limitação: ele
> reencontra o servidor pela rede (mDNS).

## Durante a transmissão

- O topo mostra o **tally real do OBS**: **● NO AR** (borda vermelha),
  **PREVIEW** (borda verde), **CONECTADA** ou **OBS SEM CONEXÃO**.
- Também aparecem: tempo transmitindo, qualidade da rede, Mb/s, resolução
  e FPS enviados, bateria (quando o navegador informa) e avisos do operador
  ("Zoom ajustado pelo operador").
- **Mantenha a página aberta e a tela ligada.** A página pede ao sistema para
  não apagar a tela, mas, se o celular bloquear ou você trocar de aplicativo, o
  navegador **pausa a câmera**. Ao voltar, ela reconecta sozinha.
- Recarregar a página durante a transmissão retoma a transmissão.
- Parar uma câmera que está **NO AR** pede confirmação.

## O que funciona no navegador

| Recurso | Chrome (Android) | Safari (iPhone) | App nativo |
|---|---|---|---|
| Vídeo H.264 + áudio Opus (WebRTC/WHIP) | Sim | Esperado* | Sim |
| Tally e comandos do operador | Sim | Esperado* | Sim |
| Zoom (local e remoto) | Quando o Chrome expõe o zoom da câmera (comum) | Depende da versão* | Sim (faixa real) |
| Troca de lente | Câmeras que o navegador lista | Câmeras que o Safari lista | Lentes reais |
| Lanterna | Quando o Chrome expõe | Normalmente não | Sim |
| Foco/exposição por toque | Quando o Chrome expõe | Não | Sim |
| Bateria na telemetria | Sim | Não | Sim |
| Temperatura do aparelho | Não | Não | Sim |
| Pinning do certificado | Não (aceite manual) | Não (aceite manual) | Sim |
| Reencontrar o servidor se o IP mudar | Não | Não | Sim (mDNS) |
| 4G/5G | Não nesta versão | Não nesta versão | Sim |

O painel só oferece um controle remoto quando o celular informa que o
recurso existe; comandos para recursos ausentes voltam como "não suportado"
— a página nunca finge que aplicou.

\* **Situação dos testes:** o fluxo completo foi testado automaticamente com
o motor do Chrome (Edge/Chrome em modo headless com câmera simulada — ver
[testes](../testing.md)). **Ainda não foi testado em celulares reais**, em
especial no Safari do iPhone, que é mais restritivo com certificados
autoassinados em conexões WebSocket.

## Problemas comuns

| Sintoma | Solução |
|---|---|
| "Permissão da câmera negada" | Nas configurações do site no navegador, permita câmera e microfone e recarregue |
| "A câmera está em uso" | Feche outros aplicativos de câmera/videochamada |
| "SEM CONEXÃO COM O PC" no topo | Celular fora do Wi-Fi da igreja, computador desligado ou firewall sem a regra TCP 7443 |
| Imagem chega mas o tally não muda | O canal de controle não conectou: recarregue a página. No iPhone, se persistir, use o app nativo |
| Página não abre | Confira o IP no painel (*Rede*) e se o celular está no mesmo Wi-Fi |
| "Este celular foi desconectado" | O aparelho foi revogado no painel: gere um novo QR Code |
