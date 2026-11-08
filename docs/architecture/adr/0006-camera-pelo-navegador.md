# ADR-0006 — Câmera pelo navegador como caminho principal

- Status: Aceito
- Data: 2026-11-08

## Contexto

Instalar o app nativo é o maior atrito para voluntários: no iPhone exige conta
Apple Developer (TestFlight/App Store); no Android, loja ou APK avulso. O pedido
foi transformar o celular em câmera **pelo navegador**, sem instalar nada.

Restrições técnicas:
- `getUserMedia` só funciona em **contexto seguro** (HTTPS ou `localhost`);
  HTTP na rede local não abre a câmera.
- Não há domínio público nem internet garantida: o certificado do servidor é
  autoassinado por instalação.
- A API WebSocket do navegador não envia o cabeçalho `Authorization`.

## Opções

1. **Página servida pela Device API (`:7443/cam/`) com o certificado atual**:
   o operador aceita o aviso do certificado uma vez por celular.
2. Certificado válido via domínio público + Let's Encrypt: exige internet,
   DNS e renovação — incompatível com o uso offline na rede local.
3. Instalar uma CA própria nos celulares: complexo demais para voluntários.
4. HTTP sem TLS: impossível (sem contexto seguro não há câmera).

## Decisão

Opção 1.
- O QR Code passa a ser um link `https://<ip>:7443/cam/#…`. Os parâmetros
  ficam no **fragmento**, que o navegador nunca envia ao servidor nem
  registra em logs/proxies, e a página o remove do histórico ao abrir. A
  câmera padrão do celular abre o link diretamente. O app nativo lê o mesmo
  QR (e ainda aceita o formato antigo `sacralink://pair?…`).
- A página é estática (HTML/CSS/JS puro, sem build e sem dependências
  externas), embutida no executável e servida com CSP estrita
  (`script-src 'self'`, `connect-src` restrito à própria origem).
- Mesma origem da API: pareamento, WHIP e canal de controle v1 sem CORS.
- WebSocket do navegador autenticado por **ticket de uso único (30 s)**
  obtido com a credencial do aparelho (`POST /device/v1/ws-ticket`).
- Mesma lógica de comandos do app (sessão, validade relativa, prioridade
  local de 3 s, idempotência) e mesma adaptação de qualidade.

## Consequências

- **Sem pinning no navegador.** A identidade do servidor depende do aceite
  do certificado do endereço lido no QR. Na rede da igreja o risco é baixo,
  mas um atacante na mesma rede que intercepte o primeiro acesso poderia
  capturar o token de pareamento. O app nativo continua com pinning e é o
  recomendado em redes não confiáveis (ver SECURITY.md).
- A credencial fica no `localStorage` da origem (`https://<ip>:7443`): se o
  IP do computador mudar, é preciso parear de novo (recomenda-se reserva
  DHCP). Pelo mesmo motivo, 4G/5G pelo navegador não é suportado nesta versão.
- Recursos dependem do navegador (zoom, lanterna, foco por toque). A página
  informa ao servidor somente as capacidades reais detectadas.
- Como o app, a câmera para com a tela bloqueada ou em segundo plano.
- O app nativo vira opcional: recomendado para quem precisa de 4G/5G, lentes
  e zoom completos, temperatura e pinning.
