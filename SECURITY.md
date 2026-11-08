# Política de Segurança

## Versões suportadas

O SacraLink está em fase pré-1.0. Apenas o branch `main` e a última release
recebem correções de segurança.

## Como reportar uma vulnerabilidade

**Não abra issues públicas** para problemas de segurança. Use o
[reporte privado de vulnerabilidades](https://github.com/gersongabrieldev/SacraLink/security/advisories/new)
do GitHub neste repositório. Informe a versão afetada, os passos para reproduzir
e o impacto. A confirmação de recebimento ocorre em até 7 dias.

## Modelo de segurança (resumo)

- A API de administração (`127.0.0.1:7380`) escuta apenas no loopback e exige a
  senha do administrador.
- A API dos dispositivos (`:7443`) usa TLS 1.3 com certificado autoassinado por
  instalação; o aplicativo móvel fixa (pinning) a impressão digital SHA-256
  recebida pelo QR Code de pareamento.
- Tokens de pareamento são de uso único, vinculados ao dispositivo e expiram em
  5 minutos. O QR Code nunca contém a senha do administrador nem credenciais
  reutilizáveis. Os dados do QR ficam no fragmento do link (`#…`), que o
  navegador não envia ao servidor; a página o remove do histórico ao abrir.
- **Câmera pelo navegador** ([ADR-0006](docs/architecture/adr/0006-camera-pelo-navegador.md)):
  o navegador **não faz pinning**. A confiança depende de o operador aceitar
  o certificado autoassinado do endereço lido no QR Code. Um atacante na mesma
  rede que intercepte esse primeiro acesso poderia capturar o token de
  pareamento (uso único, 5 min) — em redes não confiáveis use o app nativo,
  que valida a impressão digital. A página é servida com CSP estrita
  (somente scripts da própria origem, conexões só para o próprio servidor,
  sem iframes), a credencial fica no armazenamento local da origem e o
  WebSocket do navegador usa um **ticket de uso único de 30 s** em vez da
  credencial na URL.
- Segredos dos dispositivos são armazenados apenas como hash SHA-256. Qualquer
  dispositivo pode ser revogado ou bloqueado a qualquer momento.
- HTTP, RTSP, API e métricas do MediaMTX escutam apenas no loopback; somente a
  porta ICE do WebRTC é exposta. O obs-websocket nunca é exposto.
- A senha do OBS é cifrada com DPAPI do Windows (escopo do usuário atual).
- O acesso remoto (internet) vem **desativado por padrão**.

Veja o [plano técnico](docs/architecture/00-plano-tecnico.md) para o modelo de
ameaças e as decisões de projeto completas.
